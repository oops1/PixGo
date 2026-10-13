package app

func (a *App) SetActive(active bool) {
	a.Ctl.SetActive(active)
	a.Root.SetFocused(active)
}
