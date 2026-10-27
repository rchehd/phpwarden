import json
import selectors
import subprocess


class Client:
    def __init__(self, command):
        self.process = subprocess.Popen(
            command,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        self.identity = 0
        self.messages = []
        try:
            self.request(
                "initialize",
                {
                    "protocolVersion": "2025-11-25",
                    "capabilities": {},
                    "clientInfo": {"name": "phpwarden-sandbox", "version": "1"},
                },
            )
            self.process.stdin.write(
                json.dumps({"jsonrpc": "2.0", "method": "notifications/initialized"}) + "\n"
            )
            self.process.stdin.flush()
        except BaseException:
            self.process.kill()
            self.process.wait(timeout=5)
            raise

    def request(self, method, params=None):
        self.identity += 1
        payload = {"jsonrpc": "2.0", "id": self.identity, "method": method}
        if params is not None:
            payload["params"] = params
        self.process.stdin.write(json.dumps(payload) + "\n")
        self.process.stdin.flush()
        with selectors.DefaultSelector() as selector:
            selector.register(self.process.stdout, selectors.EVENT_READ)
            while selector.select(timeout=30):
                line = self.process.stdout.readline()
                if not line:
                    raise RuntimeError("MCP closed before replying")
                response = json.loads(line)
                self.messages.append(response)
                if response.get("id") == self.identity:
                    if "error" in response:
                        raise RuntimeError(f"MCP protocol error: {response['error']}")
                    return response["result"]
        raise TimeoutError("MCP reply timed out")

    def call(self, name, arguments, complete=True):
        response = self.request("tools/call", {"name": name, "arguments": arguments})
        result = response["structuredContent"]
        if bool(response.get("isError")) != (not complete) or result["complete"] != complete:
            raise AssertionError(result)
        return result

    def __enter__(self):
        return self

    def __exit__(self, kind, _value, _trace):
        try:
            self.process.stdin.close()
            code = self.process.wait(timeout=5)
            if kind is None and (code != 0 or self.process.stderr.read()):
                raise RuntimeError("MCP session failed on shutdown")
        finally:
            if self.process.poll() is None:
                self.process.kill()
                self.process.wait(timeout=5)
