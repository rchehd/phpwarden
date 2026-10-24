group "default" {
  targets = ["php"]
}

target "php" {
  name       = "php${replace(item.runtime, ".", "")}"
  dockerfile = "docker/Dockerfile"
  platforms  = ["linux/amd64"]
  matrix = {
    item = [
      { runtime = "8.1", version = "8.1.34", image = "php:8.1.34-cli-alpine3.22@sha256:51334e44f5b0c63c6b7ca34209e04f1d024179d1f14ffab9cbaf86c1c1ca05c1" },
      { runtime = "8.2", version = "8.2.35", image = "php:8.2.35-cli-alpine3.23@sha256:1539a1a6ca51f143d3801cc35f6fe6eea94f31776a77f073a2483d4433418fd7" },
      { runtime = "8.3", version = "8.3.36", image = "php:8.3.36-cli-alpine3.23@sha256:af2cec36df60396ca5e168729f38e7c8c59548a0a655fbcc362853aeda18921d" },
      { runtime = "8.4", version = "8.4.26", image = "php:8.4.26-cli-alpine3.23@sha256:da7a7a945a493ac9c015f327231afd820bbe57fc6ae22f4f520a79fbd635ba39" },
    ]
  }
  args = {
    PHP_IMAGE   = item.image
    PHP_VERSION = item.version
    PHP_RUNTIME = item.runtime
    TOOLSET_ID  = "${PHPWARDEN_TOOLSET}-php${item.runtime}"
    REQUIRE_CHECKER_VERSION = item.runtime == "8.1" ? "4.7.1" : "4.16.1"
  }
  tags = ["phpwarden:static-php${item.runtime}-dev"]
}
