// Package cdsource CommunityDragon 数据源（ADR-022）
//
// 实测验证（2026-08-28）：
//   - patch 列表: https://ddragon.leagueoflegends.com/api/versions.json -> ["16.17.1",...] 最新在前
//   - CD pin = 去掉最后一段（"16.17"；"14.24"->200，"14.24.1"->404）
//   - TFT 数据: https://raw.communitydragon.org/{pin}/cdragon/tft/{locale}.json
//   - 新 patch 发布滞后：最新 pin 404 时向下走最多 3 个 pin
package cdsource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lipipi/aurora/internal/errors"
	"github.com/lipipi/aurora/internal/game/knowledge"
)

const (
	ddragonVersionsURL = "https://ddragon.leagueoflegends.com/api/versions.json"
	cdTFTURLFormat    = "https://raw.communitydragon.org/%s/cdragon/tft/%s.json"
	maxPinFallback    = 3       // 最新 pin 404 时向下尝试的次数
	httpTimeout       = 120     // 大文件下载超时（秒）
	maxBodySize       = 64 << 20 // 64MB 上限
)

// CommunityDragon CommunityDragon 数据源
type CommunityDragon struct {
	client *http.Client
}

// New 创建数据源
func New() *CommunityDragon {
	return &CommunityDragon{
		client: &http.Client{Timeout: httpTimeout * time.Second},
	}
}

// Name 数据源名称
func (c *CommunityDragon) Name() string { return "communitydragon" }

// LatestPatch 最新可用版本 pin（如 "16.17"）
// ddragon 列表是 LOL 节奏；TFT 内容发布可能滞后 -> 逐个 pin 探测直到可用
func (c *CommunityDragon) LatestPatch(ctx context.Context) (string, errors.AppError) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ddragonVersionsURL, nil)
	if err != nil {
		return "", errors.Wrap(errors.CodeDownloadFailed, "ddragon versions request", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return "", errors.Wrap(errors.CodeDownloadFailed, "ddragon versions fetch", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", errors.Newf(errors.CodeDownloadFailed, "ddragon versions: status %d", resp.StatusCode)
	}

	var versions []string
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil || len(versions) == 0 {
		return "", errors.Wrap(errors.CodeDownloadFailed, "ddragon versions parse", err)
	}

	// 依次探测 pin（去掉最后一段），返回第一个在 CD 上存在的
	tried := 0
	for _, v := range versions {
		pin := pinOf(v)
		if pin == "" {
			continue
		}
		tried++
		if tried > maxPinFallback {
			break
		}
		if ok, _ := c.probePatch(ctx, pin, "zh_cn"); ok {
			return pin, nil
		}
	}
	return "", errors.New(errors.CodeDownloadFailed, "no usable CommunityDragon patch found")
}

// FetchTFT 下载指定版本的 TFT 数据
func (c *CommunityDragon) FetchTFT(ctx context.Context, patch, locale string, progress knowledge.ProgressFunc) ([]byte, errors.AppError) {
	if locale == "" {
		locale = "zh_cn"
	}
	url := fmt.Sprintf(cdTFTURLFormat, patch, locale)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(errors.CodeDownloadFailed, "cd tft request", err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(errors.CodeDownloadFailed, "cd tft fetch "+url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Newf(errors.CodeDownloadFailed, "cd tft %s: status %d", patch, resp.StatusCode)
	}

	total := resp.ContentLength
	if progress != nil {
		if total > 0 {
			progress(0, fmt.Sprintf("已连接，共 %.1f MB", float64(total)/1024/1024))
		} else {
			progress(-1, "下载中")
		}
	}

	limited := io.LimitReader(resp.Body, maxBodySize+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return nil, errors.Wrap(errors.CodeDownloadFailed, "cd tft read", err)
	}
	if len(buf) > maxBodySize {
		return nil, errors.New(errors.CodeDownloadFailed, "cd tft response exceeds 64MB limit")
	}
	if progress != nil {
		progress(100, fmt.Sprintf("下载完成，%.1f MB", float64(len(buf))/1024/1024))
	}
	return buf, nil
}

// probePatch 探测指定 pin + locale 是否可用（HEAD 不可靠，用小范围 GET + 立即关闭）
func (c *CommunityDragon) probePatch(ctx context.Context, pin, locale string) (bool, error) {
	url := fmt.Sprintf(cdTFTURLFormat, pin, locale)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode == http.StatusOK, nil
}

// pinOf "16.17.1" -> "16.17"（去掉最后一段）；已是两段则原样返回
func pinOf(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return ""
	}
	// 校验都是数字，防垃圾输入
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return ""
		}
	}
	return strings.Join(parts[:2], ".")
}
