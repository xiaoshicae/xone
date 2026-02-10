package xgin

import (
	"fmt"
	"strings"

	"github.com/xiaoshicae/xone/v2"
)

// PrintBanner 打印 XGin 启动 Banner（青色→紫色渐变）
func PrintBanner() {
	// 柔和渐变：雾蓝 → 灰蓝 → 淡紫
	gradientColors := [][3]int{
		{110, 180, 210},
		{112, 172, 210},
		{116, 164, 208},
		{120, 156, 206},
		{126, 148, 202},
		{134, 140, 198},
		{142, 132, 194},
	}

	lines := strings.Split(strings.TrimPrefix(bannerTxt, "\n"), "\n")
	for i, line := range lines {
		ci := i
		if ci >= len(gradientColors) {
			ci = len(gradientColors) - 1
		}
		c := gradientColors[ci]
		fmt.Printf("\x1b[38;2;%d;%d;%dm%s\x1b[0m\n", c[0], c[1], c[2], line)
	}

	// 信息行
	fmt.Printf("   \x1b[38;2;110;180;210m::\x1b[0m      \x1b[38;2;90;190;160mXGin\x1b[0m      \x1b[38;2;110;180;210m::\x1b[0m         \x1b[2m(%s RELEASE)\x1b[0m\n\n", xone.VERSION)
}

var bannerTxt = `
    __   __     _____     _____     __      _
   (_ \ / _)   / ___ \   (_   _)   /  \    / )
     \ v /    / /   \_)    | |    / /\ \  / /
      > <    ( (  ____     | |    ) ) ) ) ) )
     / ^ \   ( ( (__  )    | |   ( ( ( ( ( (
   _/ / \ \_  \ \__/ /    _| |__ / /  \ \/ /
  (__/   \__)  \____/    /_____( (_/    \__/`
