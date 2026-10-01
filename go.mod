module github.com/sourcednet/testkit

go 1.24

// Sibling projects, developed side by side in Dev/ until they are published.
replace github.com/sourcednet/core => ../core

replace github.com/sourcednet/publisher => ../publisher

require (
	github.com/sourcednet/core v0.0.0-00010101000000-000000000000
	github.com/sourcednet/publisher v0.0.0-00010101000000-000000000000
)

require (
	github.com/JohannesKaufmann/dom v0.2.0 // indirect
	github.com/JohannesKaufmann/html-to-markdown/v2 v2.4.0 // indirect
	github.com/gowebpki/jcs v1.0.2 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/text v0.28.0 // indirect
)
