// Package tft - 共享 KnowledgeProvider 接口（Phase 17-A）
//
// 背景：本接口原本在 tools/tft_knowledge.go 定义。但 Phase 17-A 把
// tools 下放到 tft/tools/ 后，tools/ 不能反向 import tft（父子互引 → cycle），
// 所以 tools 内部独立定义 KnowledgeProvider（方法集相同）。
//
// 本文件做 type alias，把 tools.KnowledgeProvider "再导出"为 tft.KnowledgeProvider，
// 让 prompt.go 等 tft 父包代码仍可写 tft.KnowledgeProvider。
//
// import 方向（Phase 17-A 终态）：
//   tft → tools            (RegisterTools 内部 type assertion)
//   tft → tfttypes         (数据访问)
//   tools → tfttypes       (数据访问)
//   tools 不 import tft    (打破 cycle)
//   tfttypes 不 import tft (数据层无业务依赖)
package tft

import (
	"github.com/lipipi/aurora/internal/game/plugins/tft/tools"
)

// KnowledgeProvider 是 TFT 插件对知识库抽象的最小接口
//
// Phase 17-A：type alias 指向 tools.KnowledgeProvider。
// 实现（如 *knowledge.Service）天然同时满足 tools / tft 两个接口（方法集相同），
// tft.RegisterTools 内做 interface→interface 转换。
type KnowledgeProvider = tools.KnowledgeProvider
