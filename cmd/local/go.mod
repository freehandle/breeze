module local

go 1.21.6

replace github.com/freehandle/breeze v0.0.0 => ../../

replace github.com/freehandle/handles v0.0.0 => ../../../handles

require (
	github.com/freehandle/breeze v0.0.0
	github.com/freehandle/handles v0.0.0
)

require github.com/freehandle/papirus v0.0.0-20240109003453-7c1dc112a42b // indirect
