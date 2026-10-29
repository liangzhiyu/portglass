package main

import (
	"fmt"
	"os"

	"github.com/egoist/mygo"
	"github.com/liangzhiyu/portglass/internal/app"
	"github.com/liangzhiyu/portglass/internal/capture"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--capture-helper" {
		if err := capture.RunHelper(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--self-test" {
		if err := app.LiveSelfTest(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	demo := false
	screenshot := ""
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "--demo" {
			demo = true
		}
		if os.Args[i] == "--screenshot" && i+1 < len(os.Args) {
			i++
			screenshot = os.Args[i]
		}
	}
	mygo.App.WhenReady(func() { app.New().Open(demo, screenshot) })
	if err := mygo.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
