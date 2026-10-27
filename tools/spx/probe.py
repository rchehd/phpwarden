import argparse
import gzip
import hashlib
import json
import os
import select
import shutil
import subprocess
import tempfile
import time
import uuid
from pathlib import Path

TOOLS = {
    "find_reports",
    "get_report_metadata",
    "get_flat_profile",
    "get_aggregated_call_graph",
    "get_callers",
}


class Reader:
    def __init__(self, image, source, reports, stderr):
        self.name = "phpwarden-spx-probe-" + uuid.uuid4().hex
        self.buffer = b""
        self.sequence = 0
        self.process = subprocess.Popen(
            [
                "docker",
                "run",
                "--rm",
                "--interactive",
                "--pull=never",
                "--name",
                self.name,
                "--network=none",
                "--read-only",
                "--memory=512m",
                "--pids-limit=64",
                "--tmpfs",
                "/tmp:rw,nosuid,nodev,size=128m",
                "--cap-drop=ALL",
                "--security-opt=no-new-privileges:true",
                "--user",
                f"{os.getuid()}:{os.getgid()}",
                "--mount",
                f"type=bind,source={source},target=/mcp,readonly",
                "--mount",
                f"type=bind,source={reports},target=/profiles,readonly",
                "--env",
                "SPX_DATA_DIR=/profiles",
                "--entrypoint",
                "php",
                image,
                "/mcp/bin/server.php",
            ],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=stderr,
        )

    def start(self):
        self.request(
            "initialize",
            {
                "protocolVersion": "2025-03-26",
                "capabilities": {},
                "clientInfo": {"name": "phpwarden-spike", "version": "0.0.0"},
            },
        )
        self.send({"jsonrpc": "2.0", "method": "notifications/initialized"})

    def send(self, message):
        self.process.stdin.write(json.dumps(message).encode() + b"\n")
        self.process.stdin.flush()

    def request(self, method, params):
        self.sequence += 1
        self.send({"jsonrpc": "2.0", "id": self.sequence, "method": method, "params": params})
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            while b"\n" in self.buffer:
                line, self.buffer = self.buffer.split(b"\n", 1)
                reply = json.loads(line)
                if reply.get("id") == self.sequence:
                    return reply
            ready, _, _ = select.select(
                [self.process.stdout], [], [], max(0, deadline - time.monotonic())
            )
            if not ready:
                break
            chunk = os.read(self.process.stdout.fileno(), 65536)
            if not chunk:
                raise RuntimeError("MCP reader exited; inspect the stderr log")
            self.buffer += chunk
            if len(self.buffer) > 1 << 20:
                raise RuntimeError("MCP response exceeds 1 MiB")
        raise TimeoutError(f"MCP request timed out: {method}")

    def raw_call(self, name, **arguments):
        return self.request("tools/call", {"name": name, "arguments": arguments})

    @staticmethod
    def failed(reply):
        return bool(reply.get("error") or reply.get("result", {}).get("isError"))

    def call(self, name, **arguments):
        reply = self.raw_call(name, **arguments)
        if self.failed(reply):
            raise RuntimeError(json.dumps(reply))
        result = reply["result"]
        if "structuredContent" in result:
            return result["structuredContent"]
        values = [json.loads(p["text"]) for p in result["content"] if p["type"] == "text"]
        return values[0] if len(values) == 1 else values

    def close(self):
        try:
            self.process.stdin.close()
            self.process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            subprocess.run(["docker", "rm", "--force", self.name], capture_output=True, timeout=10)
            self.process.wait(timeout=10)


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def probe(reader):
    advertised = reader.request("tools/list", {})
    names = {tool["name"] for tool in advertised["result"]["tools"]}
    require(names == TOOLS, "Unexpected upstream tools")
    listed = reader.call("find_reports", query="scenario.php")
    require(
        {"repeated", "cached"}.issubset({row["key"] for row in listed}),
        "Fixture reports were not discovered",
    )
    results = {"tools": sorted(names), "profiles": [], "blockers": []}
    for key, count in (("repeated", 24), ("cached", 1)):
        metadata = reader.call("get_report_metadata", report_key=key)
        flat = reader.call("get_flat_profile", report_key=key, metric="wt", limit=20)
        hot = next(row for row in flat if row["name"] == "phpwarden_expensive_lookup")
        root = next(row for row in flat if row["name"] == "/fixture/scenario.php")
        require(hot["calls"] == count, "Wrong invocation count")
        require(
            hot["file"] == "/fixture/scenario.php" and hot["lineNumber"] == 2,
            "Wrong function definition location",
        )
        require(hot["exclusiveRelative"] > 0.5, "Controlled hotspot was not found")
        graph = reader.call("get_aggregated_call_graph", report_key=key, metric="wt")
        callers = reader.call(
            "get_callers", report_key=key, function="phpwarden_expensive_lookup", metric="wt"
        )
        require("phpwarden_render_item" in json.dumps(callers), "Caller chain was lost")
        require("phpwarden_expensive_lookup" in json.dumps(graph), "Graph lost the hotspot")
        results["profiles"].append(
            {
                "key": key,
                "calls": hot["calls"],
                "wall_ns": root["inclusive"],
                "raw_wall_time_ms": metadata["wall_time_ms"],
            }
        )
    ratio = results["profiles"][0]["wall_ns"] / results["profiles"][0]["raw_wall_time_ms"]
    if 999 < ratio < 1001:
        results["blockers"].append("wall_time_ms_contains_microseconds")
    leaked = reader.call("get_report_metadata", report_key="symlink")
    if leaked.get("name") == "noisebynorthwest/php-spx-mcp":
        results["blockers"].append("metadata_symlink_escapes_report_directory")
    partial = reader.raw_call(
        "get_aggregated_call_graph",
        report_key="truncated",
        metric="wt",
        root_stack=["/fixture/scenario.php", "phpwarden_scenario"],
    )
    if not reader.failed(partial):
        results["blockers"].append("focused_graph_accepts_unclosed_stack")
    listing = reader.call("find_reports")
    if not any(row["key"] == "malformed" for row in listing):
        results["blockers"].append("malformed_metadata_omitted_without_structured_incomplete")
    traversal = reader.raw_call("get_report_metadata", report_key="../outside")
    require(reader.failed(traversal), "Path traversal was accepted")
    results["ready_for_integration"] = not results["blockers"]
    return results


def main():
    parser = argparse.ArgumentParser(description="Experimental SPX/MCP compatibility probe")
    parser.add_argument("--reader-image", required=True)
    parser.add_argument("--mcp-dir", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    args = parser.parse_args()
    directory = Path(__file__).resolve().parent
    source = args.mcp_dir.resolve(strict=True)
    pins = json.loads((directory / "sources.json").read_text())
    require(
        hashlib.sha256((source / "composer.lock").read_bytes()).hexdigest()
        == pins["mcp"]["composer_lock_sha256"],
        "Unexpected MCP dependency lock",
    )
    require((source / "vendor/autoload.php").is_file(), "Install the pinned reader dependencies")
    endpoint = os.environ.get("DOCKER_HOST")
    if not endpoint or os.environ.get("DOCKER_CONTEXT"):
        endpoint = json.loads(
            subprocess.check_output(
                ["docker", "context", "inspect", "--format", "{{json .Endpoints.docker.Host}}"],
                timeout=10,
            )
        )
    require(endpoint.startswith("unix:///"), "Probe requires a local Docker Unix socket")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="phpwarden-spx-probe-") as temporary:
        reports = Path(temporary) / "reports"
        shutil.copytree(directory / "fixtures", reports)
        metadata = (reports / "repeated.json").read_bytes()
        body = gzip.decompress((reports / "repeated.txt.gz").read_bytes()).decode()
        events, functions = body.split("[functions]", 1)
        truncated = "\n".join(events.strip().splitlines()[:-1]) + "\n[functions]" + functions
        (reports / "truncated.json").write_bytes(metadata)
        (reports / "truncated.txt.gz").write_bytes(gzip.compress(truncated.encode(), mtime=0))
        (reports / "malformed.json").write_text("{malformed")
        (reports / "malformed.txt.gz").write_bytes((reports / "cached.txt.gz").read_bytes())
        (reports / "symlink.json").symlink_to("/mcp/composer.json")
        with args.output.with_suffix(".stderr.log").open("wb") as stderr:
            reader = Reader(args.reader_image, source, reports, stderr)
            try:
                reader.start()
                result = probe(reader)
            finally:
                reader.close()
    args.output.write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
