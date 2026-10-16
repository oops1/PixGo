package view

import "github.com/oops1/PixGo/internal/viewer"

const panStepDivisor = 10

func (c *Canvas) PanSteps(right, down int) {
	if !c.CanPan() {
		return
	}
	dx := -float64(right) * float64(c.areaW) / panStepDivisor
	dy := -float64(down) * float64(c.areaH) / panStepDivisor
	c.apply(func(f viewer.Frame) { c.view.Pan(f, dx, dy) })
}
