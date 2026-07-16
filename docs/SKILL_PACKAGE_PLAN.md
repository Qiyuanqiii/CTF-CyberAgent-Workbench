# CyberAgent Workbench Custom Skill Package Plan

状态：格式基础部分已完成，用户导入/上传通道尚未实现。

## 当前能力

现有 Go `skill.v1` 已严格定义名称、语义版本、描述、兼容 Profile、工具前置声明、Markdown 内容路径、UTF-8 字节数、保守 token 上界和 SHA-256。`internal/skills.LoadFS` 可以从受控 `fs.FS` 构造不可变 Registry，当前只用于编译进二进制的 `code`、`review`、`learn`、`script` 和 `plan-delivery`，以及测试夹具。

当前产品入口只有：

- `cyberagent skill list`
- `cyberagent skill show`
- `cyberagent skill validate`
- `cyberagent skill select`
- `cyberagent skill selection`

因此项目目前没有 `skill import/install/upload`，没有标准 ZIP 技能包、用户 Registry、签名/来源账本、HTTP 上传端点或桌面文件选择入口。内部 `LoadFS` 不能被描述为用户导入功能。

## 第一版包边界

- 定义 `skill_package.v1`，第一版只允许一个严格 `skill.v1` Manifest 和一份 UTF-8 Markdown 指导正文。
- 包使用确定性 ZIP 或等价只读字节容器；实现前通过 ADR 固定根目录、文件顺序、压缩方法和 package fingerprint。
- 禁止绝对路径、`..`、反斜杠歧义、重复/大小写碰撞条目、symlink、hardlink、设备、FIFO、嵌套压缩包、尾随数据和 ZIP bomb。
- 限制压缩包字节、解压字节、文件数、目录深度、单文件大小和压缩比；所有限制在读取正文前执行。
- 第一版不接受可执行脚本、二进制、动态库、任意资源下载或安装钩子。后续脚本资源仍只能作为 Artifact，由 Go Tool Gateway、Policy、Scope 和审批独立授权。

## 信任与权限

- 包内容是操作者安装的工作流指导，不是系统策略，也不能覆盖 Policy、Scope、预算、Approval、Sandbox 或来源隔离。
- `tool_dependencies` 仍只是前置声明，永不授予工具能力。
- 导入不会执行正文、脚本、命令、网络请求或 Provider 调用。
- 外部包默认标记为 `operator_installed_untrusted`；只有操作者显式安装并为 Run 选择后才能进入模型上下文。
- 上下文交付仍需版本/hash/bytes/Profile 精确复核、secret redaction、独立 token 预算和 capability=false 来源事实。
- Code 与 Cyber Catalog 分离。Cyber 默认不继承 Code Skill；Script Profile 只能使用明确兼容的窄化脚本指导。

## 本地 Registry

- Go 在用户数据目录维护 content-addressed、不可变的 Skill 版本目录；正文不进入仓库，也不进入 SQLite 事件。
- SQLite 只保存包摘要、来源类型、信任状态、安装操作、Profile 和版本引用等有界元数据。
- 相同内容幂等收敛；同名同版本不同内容永久冲突；同一 Skill 最多保留有界版本数。
- 已被 Run 固定选择的版本不能删除。卸载只影响未来选择，历史 Run 必须仍能精确恢复或明确报告缺失，不能静默升级。
- 导入事务使用临时目录、完整验证、fsync/原子 rename 和崩溃恢复；失败不得留下半安装 Registry。

## 产品入口

CLI 第一阶段：

```text
cyberagent skill package validate <package.zip>
cyberagent skill import <package.zip> --operation-key <stable-key> --confirm-untrusted-skill
cyberagent skill installed [--surface code|cyber] [--profile <profile>]
cyberagent skill remove <name>@<version> --operation-key <stable-key>
```

Desktop D1 阶段：

- 文件选择器只把一个受限本地文件句柄/路径交给 Go；TypeScript 不解压、不校验、不写 Registry。
- Go 先返回 metadata-only 验证预览、风险代码、Profile、声明工具和 package digest，再要求显式确认安装。
- 上传端点只能绑定回环地址，使用独立 control token、严格 Origin/Host、固定 Content-Type、流式大小上限、幂等 operation key 和无正文审计事件。
- Web 远程上传、URL 安装、Git 仓库安装和在线 Marketplace 均后置，不允许第一版自动下载并信任内容。

## 签名与分发

- 本地未签名包可以在明确警告后安装，但保持 untrusted 标记。
- 签名包需要独立 publisher identity、签名算法、证书轮换、吊销和透明来源设计；签名只证明发布者和完整性，不授予能力。
- 官方内置 Skill 与用户安装 Skill 使用不同 trust class 和 Catalog，不允许用户包覆盖内置名称/版本。

## 实施切片

1. 固定 `skill_package.v1` 和威胁模型，完成纯内存 parser/validator/fuzzer，不写磁盘。
2. 增加 content-addressed 本地 Registry、不可变安装账本、原子导入/卸载和 CLI。
3. 将用户 Skill 纳入 Run 选择、root/Specialist 最小上下文和 Code/Cyber 分离测试。
4. 在 Desktop D1 增加 Go-owned 文件选择、验证预览和确认安装；HTTP mutation 必须单独审计。
5. 最后评估签名包、团队 Catalog 与 Marketplace；远程自动安装不属于基础版本。

## 验收标准

- 恶意 ZIP、路径逃逸、链接、特殊文件、超大包、重复项、Unicode/大小写碰撞和内容 hash 漂移全部失败关闭。
- 导入前后不会执行命令、访问网络、调用模型或改变工具权限。
- 并发导入、重启恢复和相同 operation 重放收敛到同一不可变版本。
- Run 固定版本可跨重启恢复，包升级/卸载不会改变已开始 Run 的上下文。
- CLI 与 Desktop 对同一包产生相同 digest、风险预览、错误码和安装结果。
