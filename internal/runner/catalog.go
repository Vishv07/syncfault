package runner

import (
	"github.com/Vishv07/syncfault/internal/adapter"
	"github.com/Vishv07/syncfault/internal/fault"
	"github.com/Vishv07/syncfault/internal/scenario"
)

// Recipes lists the registered workload recipes.
func Recipes() []string { return scenario.RecipeNames() }

// Faults lists the registered fault injectors, including "none".
func Faults() []string { return fault.Names() }

// Adapters lists the registered target adapters.
func Adapters() []string { return adapter.Names() }
