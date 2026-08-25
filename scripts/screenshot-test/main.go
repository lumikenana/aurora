package main

import (
	"context"
	"fmt"
	"time"

	"github.com/lipipi/game-ai/internal/screenshot"
)

func main() {
	p, err := screenshot.CreateProvider(screenshot.DefaultConfig())
	if err != nil {
		fmt.Println("create provider failed:", err)
		return
	}
	fmt.Println("provider:", p.Name(), "displays:", p.GetDisplayCount())

	req := screenshot.ScreenshotRequest{
		Mode:     screenshot.CaptureModeFullScreen,
		MaxWidth: 1920,
		MaxHeight: 1080,
		Quality:  80,
	}

	done := make(chan struct{})
	go func() {
		start := time.Now()
		s, err := p.Capture(context.Background(), req)
		if err != nil {
			fmt.Println("capture failed:", err)
		} else {
			fmt.Printf("capture ok: %dx%d, %d bytes, took %v\n", s.Width, s.Height, len(s.Bytes), time.Since(start))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		fmt.Println("TIMEOUT: capture did not return in 10s (deadlock in GDI call)")
	}
}
