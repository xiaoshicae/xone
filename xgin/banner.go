package xgin

import (
	"fmt"

	"github.com/xiaoshicae/xone"
)

func PrintBanner() {
	fmt.Println(bannerTxt)
	coloredName := fmt.Sprintf("\x1b[32m%s\x1b[0m", "::      XGin      ::")
	fmt.Printf("   %s         (%s RELEASE)\n\n", coloredName, xone.VERSION)
}

var bannerTxt = `
    __   __     _____     _____     __      _
   (_ \ / _)   / ___ \   (_   _)   /  \    / )
     \ v /    / /   \_)    | |    / /\ \  / /
      > <    ( (  ____     | |    ) ) ) ) ) )
     / ^ \   ( ( (__  )    | |   ( ( ( ( ( (
   _/ / \ \_  \ \__/ /    _| |__ / /  \ \/ /
  (__/   \__)  \____/    /_____( (_/    \__/`
