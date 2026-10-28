package views

// Chart inputs belong to their view, which clears the output on data or style
// changes. Retain only one bounded string per panel, never the backing canvas.
const maxChartRenderBytes = 1 << 20

type chartRenderCache struct {
	width  int
	height int
	output string
	ready  bool
}

func (c chartRenderCache) get(width, height int) (string, bool) {
	return c.output, c.ready && c.width == width && c.height == height
}

func (c *chartRenderCache) store(width, height int, output string) string {
	c.clear()
	if len(output) <= maxChartRenderBytes {
		*c = chartRenderCache{width: width, height: height, output: output, ready: true}
	}
	return output
}

func (c *chartRenderCache) clear() {
	*c = chartRenderCache{}
}
