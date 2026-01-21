package views

func framedTableSize(width, height int) (int, int) {
	return width - 4, max(height-2, 3)
}
