# CyberAgent Deterministic Analyzers

## 中文

此目录承载由 Go 控制平面定义协议、由 Rust 实现的确定性分析工具。Rust 不是 Agent，
不管理 Run、Session、LLM、用户配置、API key、审批、Docker、网络或持久化。

当前 `cyberagent-analyzer-fixture` 只实现 `analyzer_protocol.v1` 的开发期夹具：

- 从 stdin 读取最多 96 KiB 的严格 JSON；
- 只接受 Base64 内联输入，不接受路径、URL 或命令；
- filesystem、network、subprocess、environment 四类能力必须全部为 `false`；
- 向 stdout 输出最多 16 KiB 的 metadata-only 结果或稳定错误信封；
- 不返回原始内容，只返回媒体类型、字节数、SHA-256、UTF-8 与逻辑行数；
- 不被 CLI、HTTP、Desktop、Tool Gateway、Runner 或 Artifact 流程调用。

Go 和 Rust 分别读取
[`testdata/analyzer_protocol_v1_vectors.json`](testdata/analyzer_protocol_v1_vectors.json)，
独立校验版本、限制、错误码、退出码、JSON 语义及 stdout 的精确 bytes/SHA-256。

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
`analyzer_protocol.v1` reference. It accepts only bounded inline stdin JSON, requires all
external capabilities to remain disabled, emits bounded metadata-only stdout JSON, and
has no product invocation or Artifact-commit path. Go and Rust independently validate
the same versioned golden vectors, including exact output bytes and SHA-256.
