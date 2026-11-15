package engine

// Closed reports whether the App has released its resources. A nil App is
// closed. Like other App methods, call Closed on the application loop goroutine.
func (app *App) Closed() bool { return app == nil || app.closed }
