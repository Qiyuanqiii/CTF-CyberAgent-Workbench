# CyberAgent Deterministic Analyzers

## 中文

此目录承载由 Go 控制平面定义协议、由 Rust 实现的确定性分析工具。Rust 不是 Agent，
不管理 Run、Session、LLM、用户配置、API key、审批、Docker、网络或持久化。

当前 `cyberagent-analyzer-fixture` 实现 `analyzer_protocol.v1` 的两项开发期纯函数：

- 从 stdin 读取最多 96 KiB 的严格 JSON；
- 只接受 Base64 内联输入，不接受路径、URL 或命令；
- filesystem、network、subprocess、environment 四类能力必须全部为 `false`；
- 向 stdout 输出最多 16 KiB 的 metadata-only 结果或稳定错误信封；
- `fixture.digest.v1` 不返回原始内容，只返回媒体类型、字节数、SHA-256、UTF-8 与逻辑行数；
- `archive.zip.inventory.v1` 只遍历内存 ZIP 的中央目录，不打开条目正文、不解压、不写文件；
- 不被 CLI、HTTP、Desktop、Tool Gateway、Runner 或 Artifact 流程调用。

Go 的只读 `analyzer_descriptor.v1` Registry 目前固定两项 descriptor，且没有动态注册、
executable、command、path 或 starter 字段。ZIP inventory 最多接受 32 个条目、单个 128 字节
名称和合计 2 KiB 名称；8 MiB 单项声明尺寸、32 MiB 合计声明尺寸和 100:1 压缩比属于明确风险
阈值，不会触发解压。路径穿越、绝对路径、反斜杠、重名、目录携带数据及尺寸/压缩比异常都以
排序后的稳定风险码返回。

Rust 固定使用 `rawzip = 0.5.1` 读取中央目录记录。该 crate 为 MIT 许可、无传递依赖，并在源码
中 `forbid(unsafe_code)`。Rust 不调用本地文件 API；ZIP 字节仍只来自 Go-owned 请求中的 Base64
内联内容。

Go 和 Rust 分别读取
[`testdata/analyzer_protocol_v1_vectors.json`](testdata/analyzer_protocol_v1_vectors.json)，
独立校验版本、限制、错误码、退出码、JSON 语义及 stdout 的精确 bytes/SHA-256。
两种语言还分别读取
[`testdata/archive_inventory_v1_vectors.json`](testdata/archive_inventory_v1_vectors.json)，
校验正常包、路径穿越、重名、声明尺寸过大和压缩比异常五组固定 ZIP 输入及结果。

```powershell
$env:PATH = "$env:USERPROFILE\.cargo\bin;$env:PATH"
Set-Location analyzers
cargo fmt --all -- --check
cargo test --locked
cargo clippy --locked --all-targets -- -D warnings
```

## English

This directory contains deterministic Rust analyzers behind a Go-owned protocol. Rust is
not an Agent and does not own Runs, Sessions, LLM calls, user configuration, API keys,
approvals, Docker, networking, or persistence.

The current `cyberagent-analyzer-fixture` is a development-only
`analyzer_protocol.v1` reference with `fixture.digest.v1` and
`archive.zip.inventory.v1`. The latter uses pinned `rawzip 0.5.1` to iterate only an
in-memory ZIP central directory. It never opens entry content, decompresses, extracts,
or writes a file. Entry count, name bytes, declared sizes, integer compression ratio,
and path risks are bounded and deterministic.

Go owns an inert, fixed `analyzer_descriptor.v1` Registry with no dynamic registration,
executable, command, path, or starter field. All capabilities and product-authority bits
remain false. Go and Rust independently validate both versioned golden-vector files,
including five fixed ZIP inputs plus exact output JSON bytes and SHA-256. There is still
no CLI, HTTP, Desktop, Runner, Run/Event/SQLite, persistence, or Artifact-commit path.
