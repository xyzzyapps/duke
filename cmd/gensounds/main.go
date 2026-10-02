// Command gensounds writes the six sound-effect WAV samples into a
// directory (default: sounds/) for the sample-based audio engine to play.
// It is an offline asset generator: the game itself never synthesises.
package main

import (
	"flag"
	"log"

	"shooter/internal/audio"
)

func main() {
	dir := flag.String("dir", audio.SoundsDir, "output directory for WAV samples")
	flag.Parse()
	if err := audio.GenerateSamples(*dir); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote samples to %s: slash, pistol, shotgun, rocket, stamp, save, step (.wav)", *dir)
}
