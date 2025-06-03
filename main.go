package main

func main() {
	app, ok := NewApp()
	if ok {
		<-app.Done()
	}
}
