module github.com/freehandle/breeze

replace github.com/freehandle/papirus => ../papirus

go 1.20

require (
	github.com/freehandle/cb v0.0.0-20231208135312-0344e092a799
	github.com/freehandle/papirus v0.0.0-00010101000000-000000000000
	golang.org/x/term v0.15.0
)

require golang.org/x/sys v0.15.0 // indirect
