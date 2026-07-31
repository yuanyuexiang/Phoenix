# Phoenix shared contracts

这里的 JSON Schema 是 WorkBuddy、Go 服务端和管理端 UI 的共享边界。首期保持
`/pub/v1` 向后兼容：原有 `{name,value,confidence}` 字段仍有效，证据通过可选的
`evidence` 扩展。

- `extraction.schema.json`：字段值及其文档/计算/人工证据。
- `finding.schema.json`：数据质量或业务策略发现。
- `action.schema.json`：动作预览与执行结果的稳定外形。

示例位于 `examples/`，CI 和三端测试应优先复用这些样例。
