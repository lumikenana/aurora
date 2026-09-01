// knowledge-test 知识库独立验证程序（沿 screenshot-test 先例）
//
// 验证内容：
//  1. fixture 导入（双版本 A/B）
//  2. 版本隔离：active=A 时只返回 A 的数据
//  3. 重导入 A 只替换 A，B 不受影响
//  4. 模糊匹配（大小写不敏感子串）
//  5. envelope 手动导入（缺 patch 拒绝）
//  6. -download 标志：实网下载 CommunityDragon 最新数据冒烟
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lipipi/aurora/internal/game"
	"github.com/lipipi/aurora/internal/game/knowledge"
	"github.com/lipipi/aurora/internal/game/knowledge/cdsource"
	"github.com/lipipi/aurora/internal/game/knowledge/store"
)

var failed bool

func check(name string, cond bool, detail string) {
	if cond {
		fmt.Printf("  ✓ %s\n", name)
	} else {
		failed = true
		fmt.Printf("  ✗ %s: %s\n", name, detail)
	}
}

func main() {
	download := flag.Bool("download", false, "实网下载 CommunityDragon 最新数据冒烟")
	flag.Parse()

	dir, err := os.MkdirTemp("", "knowledge-test-*")
	if err != nil {
		fmt.Println("temp dir failed:", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	fmt.Println("test dir:", dir)

	st, err := store.Open(filepath.Join(dir, "knowledge.db"))
	if err != nil {
		fmt.Println("open store failed:", err)
		os.Exit(1)
	}
	defer st.Close()

	ctx := context.Background()

	// ---- 1. fixture 导入双版本 ----
	fmt.Println("\n[1] fixture 导入（版本 A=16.1 / B=16.2）")
	impA := knowledge.NormalizedPatchData{
		Game: "tft", Patch: "16.1", Source: "manual",
		Champions: []knowledge.Record{
			{ID: "TFT16_Jinx", Name: "金克丝", Data: []byte(`{"cost":1}`)},
			{ID: "TFT16_Vi", Name: "蔚", Data: []byte(`{"cost":2}`)},
		},
		Items:    []knowledge.Record{{ID: "TFT_Item_Bow", Name: "反曲之弓", Data: []byte(`{}`)}},
		Traits:   []knowledge.Record{{ID: "TFT16_Gunner", Name: "枪手", Data: []byte(`{}`)}},
	}
	impB := impA
	impB.Patch = "16.2"
	impB.Champions = []knowledge.Record{
		{ID: "TFT16_Jinx", Name: "金克丝", Data: []byte(`{"cost":2}`)}, // 数值改动
		{ID: "TFT16_Ekko", Name: "艾克", Data: []byte(`{"cost":4}`)},  // 新英雄
	}

	sA, err := st.ImportNormalized(ctx, impA)
	check("导入 16.1", err == nil && sA.Champions == 2 && sA.Items == 1 && sA.Traits == 1, fmt.Sprintf("summary=%+v err=%v", sA, err))
	sB, err := st.ImportNormalized(ctx, impB)
	check("导入 16.2", err == nil && sB.Champions == 2, fmt.Sprintf("summary=%+v err=%v", sB, err))

	// ---- 2. 版本隔离 ----
	fmt.Println("\n[2] 版本隔离")
	svc := knowledge.NewService(st, nil, game.KnowledgeConfig{Enabled: true, Locale: "zh_cn"}, filepath.Join(dir, "raw"))
	patch, aerr := svc.GetCurrentPatch(ctx)
	check("active patch = 最新 16.2", aerr == nil && patch == "16.2", fmt.Sprintf("patch=%q err=%v", patch, aerr))

	champs, aerr := svc.GetChampion(ctx, "tft", knowledge.Query{})
	check("查询限定 active patch（艾克在 16.2）", aerr == nil && len(champs) == 2, fmt.Sprintf("n=%d err=%v", len(champs), aerr))
	jinxPatch := ""
	for _, c := range champs {
		if c.ID == "TFT16_Jinx" {
			jinxPatch = c.Patch
		}
	}
	check("金克丝记录带 16.2 patch 标签", jinxPatch == "16.2", "patch="+jinxPatch)

	// 直接用 store 查旧版本仍完整保留
	oldChamps, _ := st.QueryChampion(ctx, "tft", "16.1", knowledge.Query{})
	check("16.1 数据仍完整保留（5 条记录不被覆盖）", len(oldChamps) == 2, fmt.Sprintf("n=%d", len(oldChamps)))

	// ---- 3. 重导入只替换该版本 ----
	fmt.Println("\n[3] 重导入 16.1 只替换 16.1")
	impA2 := impA
	impA2.Champions = []knowledge.Record{{ID: "TFT16_New", Name: "新英雄", Data: []byte(`{}`)}}
	_, err = st.ImportNormalized(ctx, impA2)
	check("重导入 16.1 成功", err == nil, fmt.Sprintf("err=%v", err))
	oldChamps, _ = st.QueryChampion(ctx, "tft", "16.1", knowledge.Query{})
	check("16.1 只剩新记录（旧记录被替换）", len(oldChamps) == 1 && oldChamps[0].Name == "新英雄", fmt.Sprintf("n=%d", len(oldChamps)))
	newChamps, _ := st.QueryChampion(ctx, "tft", "16.2", knowledge.Query{})
	check("16.2 不受影响", len(newChamps) == 2, fmt.Sprintf("n=%d", len(newChamps)))

	// ---- 4. 模糊匹配 ----
	fmt.Println("\n[4] 模糊匹配")
	champs, aerr = svc.GetChampion(ctx, "tft", knowledge.Query{Name: "金克"})
	check("模糊匹配 '金克' -> 金克丝", aerr == nil && len(champs) == 1 && champs[0].Name == "金克丝", fmt.Sprintf("n=%d err=%v", len(champs), aerr))
	items, aerr := svc.GetItem(ctx, "tft", knowledge.Query{Name: "之弓"})
	check("模糊匹配 '之弓' -> 反曲之弓", aerr == nil && len(items) == 1 && items[0].Name == "反曲之弓", fmt.Sprintf("n=%d err=%v", len(items), aerr))
	byItemID, aerr := svc.GetItem(ctx, "tft", knowledge.Query{ID: "TFT_Item_Bow"})
	check("装备 ID 精确匹配", aerr == nil && len(byItemID) == 1, fmt.Sprintf("n=%d err=%v", len(byItemID), aerr))
	byID, aerr := svc.GetChampion(ctx, "tft", knowledge.Query{ID: "TFT16_Ekko"})
	check("ID 精确匹配 -> 艾克", aerr == nil && len(byID) == 1 && byID[0].Name == "艾克", fmt.Sprintf("n=%d err=%v", len(byID), aerr))

	// ---- 5. envelope 手动导入 ----
	fmt.Println("\n[5] envelope 手动导入")
	env := []byte(`{"game":"tft","patch":"16.2","kind":"compositions","data":[
		{"id":"comp-1","name":"枪手艾克","tier":"S","champions":["艾克","金克丝"],"description":"测试阵容"}]}`)
	d, perr := knowledge.ParseEnvelope(env)
	check("envelope 解析成功", perr == nil && d != nil && len(d.Compositions) == 1, fmt.Sprintf("err=%v", perr))
	if d != nil {
		_, ierr := st.ImportNormalized(ctx, *d)
		check("compositions 导入成功", ierr == nil, fmt.Sprintf("err=%v", ierr))
		comps, _ := svc.GetComposition(ctx, "tft", knowledge.Query{Name: "艾克"})
		check("阵容查询命中且带版本", len(comps) == 1 && comps[0].Patch == "16.2", fmt.Sprintf("n=%d", len(comps)))
	}
	bad := []byte(`{"game":"tft","kind":"champions","data":[{"id":"x","name":"无版本"}]}`)
	_, perr = knowledge.ParseEnvelope(bad)
	check("缺 patch 的导入被拒绝", perr != nil, "应该报错但没有")

	// ---- 6. 状态 ----
	fmt.Println("\n[6] 状态汇总")
	status, serr := svc.Status(ctx)
	check("Status 返回", serr == nil && status != nil && status.ActivePatch == "16.2" && status.HasData, fmt.Sprintf("status=%+v err=%v", status, serr))
	if status != nil {
		fmt.Printf("    active=%s patches=%v counts=%v\n", status.ActivePatch, status.Patches, status.Counts)
	}

	// ---- 7. 实网下载冒烟（可选） ----
	if *download {
		fmt.Println("\n[7] CommunityDragon 实网下载冒烟")
		src := cdsource.New()
		dl := knowledge.NewService(st, src, game.KnowledgeConfig{Enabled: true, Locale: "zh_cn"}, filepath.Join(dir, "raw"))
		progress := func(p int, msg string) { fmt.Printf("    [%d%%] %s\n", p, msg) }
		summary, derr := dl.DownloadLatest(ctx, progress)
		check("下载并导入成功", derr == nil, fmt.Sprintf("err=%v", derr))
		if derr == nil {
			fmt.Printf("    summary: %+v\n", summary)
			gp, _ := dl.GetCurrentPatch(ctx)
			fmt.Printf("    当前版本: %s\n", gp)
			jx, _ := dl.GetChampion(ctx, "tft", knowledge.Query{Name: "金克丝"})
			for _, c := range jx {
				fmt.Printf("    金克丝: id=%s patch=%s data=%s\n", c.ID, c.Patch, truncate(string(c.Data), 120))
			}
		}
	}

	fmt.Println()
	if failed {
		fmt.Println("结果: 存在失败项 ✗")
		os.Exit(1)
	}
	fmt.Println("结果: 全部通过 ✓")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
