# API reference

The panel's HTTP API is described by [`api/openapi.yaml`](https://github.com/minelifes/nectar_apps/blob/main/api/openapi.yaml)
(OpenAPI 3.1). Authenticate with an API token: `Authorization: Bearer nat_…`. Each endpoint lists the
permission it needs as `x-permission`.

<div id="redoc-container"></div>
<script src="https://cdn.redoc.ly/redoc/v2.1.5/bundles/redoc.standalone.js"></script>
<script>
  Redoc.init("../openapi.yaml", { hideDownloadButton: false, expandResponses: "200", theme: { colors: { primary: { main: "#f59e0b" } } } },
    document.getElementById("redoc-container"));
</script>

[Download openapi.yaml](openapi.yaml)
