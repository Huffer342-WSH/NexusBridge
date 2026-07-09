// main.go 负责组装 Wails v3 桌面窗口、托盘和共享后端服务。
package main

import (
	"context"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"nexusbridge/internal/desktop"
	webuiassets "nexusbridge/webui"
)

// main 启动与浏览器 WebUI 共用业务核心的桌面应用。
func main() {
	icon := desktopIcon()

	var mainWindow *application.WebviewWindow
	var desktopRuntime *desktop.Runtime
	apiSlot := &desktop.HandlerSlot{}
	app := application.New(application.Options{
		Name:        "NexusBridge Desktop",
		Description: "NexusPHP 种子检索、qBittorrent 同步与媒体整理工具",
		Icon:        icon,
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(webuiassets.Dist()),
			Middleware: application.Middleware(desktop.APIMiddleware(apiSlot)),
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.nexusbridge.desktop",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				showMainWindow(mainWindow)
			},
		},
		OnShutdown: func() {
			if desktopRuntime != nil {
				if err := desktopRuntime.Close(); err != nil {
					log.Printf("关闭桌面服务失败: %v", err)
				}
			}
		},
	})
	desktopRuntime, err := desktop.Start(context.Background(), desktop.Options{})
	if err != nil {
		log.Fatal(err)
	}
	apiSlot.Set(desktopRuntime.Handler())

	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "NexusBridge Desktop",
		Width:            1200,
		Height:           800,
		MinWidth:         900,
		MinHeight:        640,
		URL:              "/",
		BackgroundColour: application.NewRGB(244, 246, 251),
	})
	mainWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		mainWindow.Hide()
		event.Cancel()
	})

	tray := app.SystemTray.New()
	tray.SetIcon(icon)
	tray.SetTooltip("NexusBridge Desktop")
	tray.OnClick(func() {
		showMainWindow(mainWindow)
	})
	trayMenu := app.NewMenu()
	trayMenu.Add("显示主窗口").OnClick(func(*application.Context) {
		showMainWindow(mainWindow)
	})
	trayMenu.AddSeparator()
	trayMenu.Add("完全退出").OnClick(func(*application.Context) {
		app.Quit()
	})
	tray.SetMenu(trayMenu)

	if err := app.Run(); err != nil {
		_ = desktopRuntime.Close()
		log.Fatal(err)
	}
}

// showMainWindow 显示并聚焦桌面主窗口。
func showMainWindow(window *application.WebviewWindow) {
	if window == nil {
		return
	}
	window.Show()
	window.Focus()
}
