package piplayer

import "os"

// exists checks if a directory exists. Anything that isn't a plain "not
// there" - a permission problem, a broken network mount - counts as missing
// too, because the player can't read it either way.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
