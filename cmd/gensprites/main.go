// Command gensprites exports the built-in character sheets (Duke, Stick)
// as editable PNGs into sprites/<name>/<frame>.png. Run it after changing
// the ASCII art; the game then loads the images (sprites/ takes priority).
package main

import (
	"fmt"
	"log"

	"shooter/internal/render"
)

func main() {
	if err := render.ExportBuiltinSheets("sprites"); err != nil {
		log.Fatal(err)
	}
	fmt.Println("wrote sprites/duke and sprites/stick (*.png)")
}
