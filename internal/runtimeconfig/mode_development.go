// mode_development.go 定义非 production 构建的数据目录策略。
//go:build !production

package runtimeconfig

const productionBuild = false
