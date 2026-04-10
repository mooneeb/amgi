package main

import (
	"github.com/mooneeb/amgi/internal/config/validate"
)

func main() {
	validate.ValidateConfig("examples/correct.yaml")
}
