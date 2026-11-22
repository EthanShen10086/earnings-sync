# packages/api-client

浏览器友好的 ESM 客户端，对齐 `GET /v1/meta`、`/v1/facts`、`/v1/auth/me`、`/v1/platform/status`。

```js
import { createClient } from "./index.js";

const api = createClient({ baseURL: "http://127.0.0.1:8090" });
const meta = await api.meta();
const { facts } = await api.facts("AAPL");
```

`apps/web` 通过 `<script type="module">` 引用本包；无构建步骤。
