package coffee

import (
	"fmt"
	"strings"
	"time"

	coffeedata "thom-server/internal/coffee"
)

type coffeeEntryPatch struct {
	Date             *string  `json:"date"`
	CoffeeName       *string  `json:"coffeeName"`
	Origin           *string  `json:"origin"`
	CoffeeVarietal   *string  `json:"coffeeVarietal"`
	ProcessingMethod *string  `json:"processingMethod"`
	DaysSinceRoast   *int     `json:"daysSinceRoast"`
	RoasterID        *string  `json:"roasterId"`
	Roaster          *string  `json:"roaster"`
	BrewMethod       *string  `json:"brewMethod"`
	Ratio            *string  `json:"ratio"`
	Grinder          *string  `json:"grinder"`
	GrinderID        *string  `json:"grinderId"`
	GrindSetting     *float64 `json:"grindSetting"`
	Dose             *int     `json:"dose"`
	YieldAmount      *int     `json:"yieldAmount"`
	WaterTemperature *int     `json:"waterTemperature"`
	BrewTime         *string  `json:"brewTime"`
	BloomTime        *string  `json:"bloomTime"`
	BloomWater       *int     `json:"bloomWater"`
	PourNotes        *string  `json:"pourNotes"`
	RoastLevel       *string  `json:"roastLevel"`
	Notes            *string  `json:"notes"`
	TastingNotes     *string  `json:"tastingNotes"`
	Rating           *int     `json:"rating"`
}

// validCoffeeEntryPatch requires a roaster or grinder to be patched by both its
// id and its name, so the lookup pair cannot be left half-updated.
func validCoffeeEntryPatch(patch *coffeeEntryPatch) bool {
	if (patch.RoasterID == nil) != (patch.Roaster == nil) {
		return false
	}
	if (patch.GrinderID == nil) != (patch.Grinder == nil) {
		return false
	}

	return true
}

func normalizeCoffeeEntry(entry *coffeedata.Entry) {
	entry.Date = strings.TrimSpace(entry.Date)
	entry.CoffeeName = strings.TrimSpace(entry.CoffeeName)
	entry.Origin = strings.TrimSpace(entry.Origin)
	entry.CoffeeVarietal = strings.TrimSpace(entry.CoffeeVarietal)
	entry.ProcessingMethod = strings.TrimSpace(entry.ProcessingMethod)
	entry.RoasterID = strings.TrimSpace(entry.RoasterID)
	entry.Roaster = strings.TrimSpace(entry.Roaster)
	entry.BrewMethod = strings.TrimSpace(entry.BrewMethod)
	entry.Ratio = strings.TrimSpace(entry.Ratio)
	entry.GrinderID = strings.TrimSpace(entry.GrinderID)
	entry.Grinder = strings.TrimSpace(entry.Grinder)
	entry.BrewTime = strings.TrimSpace(entry.BrewTime)
	entry.BloomTime = strings.TrimSpace(entry.BloomTime)
	entry.PourNotes = strings.TrimSpace(entry.PourNotes)
	entry.RoastLevel = strings.TrimSpace(entry.RoastLevel)
	entry.Notes = strings.TrimSpace(entry.Notes)
	entry.TastingNotes = strings.TrimSpace(entry.TastingNotes)

	if entry.Notes == "" {
		entry.Notes = entry.TastingNotes
	}
	entry.TastingNotes = entry.Notes

	if entry.RoasterID == "" && entry.Roaster != "" {
		entry.RoasterID = coffeedata.SlugifyName(entry.Roaster)
	}
	if entry.GrinderID == "" && entry.Grinder != "" {
		entry.GrinderID = coffeedata.SlugifyName(entry.Grinder)
	}
}

func applyCoffeeEntryPatch(entry *coffeedata.Entry, patch *coffeeEntryPatch) {
	if patch.Date != nil {
		entry.Date = *patch.Date
	}
	if patch.CoffeeName != nil {
		entry.CoffeeName = *patch.CoffeeName
	}
	if patch.Origin != nil {
		entry.Origin = *patch.Origin
	}
	if patch.CoffeeVarietal != nil {
		entry.CoffeeVarietal = *patch.CoffeeVarietal
	}
	if patch.ProcessingMethod != nil {
		entry.ProcessingMethod = *patch.ProcessingMethod
	}
	if patch.DaysSinceRoast != nil {
		entry.DaysSinceRoast = *patch.DaysSinceRoast
	}
	if patch.RoasterID != nil {
		entry.RoasterID = *patch.RoasterID
	}
	if patch.Roaster != nil {
		entry.Roaster = *patch.Roaster
		if patch.RoasterID == nil {
			entry.RoasterID = ""
		}
	}
	if patch.BrewMethod != nil {
		entry.BrewMethod = *patch.BrewMethod
	}
	if patch.Ratio != nil {
		entry.Ratio = *patch.Ratio
	}
	if patch.Grinder != nil {
		entry.Grinder = *patch.Grinder
		if patch.GrinderID == nil {
			entry.GrinderID = ""
		}
	}
	if patch.GrinderID != nil {
		entry.GrinderID = *patch.GrinderID
	}
	if patch.GrindSetting != nil {
		entry.GrindSetting = *patch.GrindSetting
	}
	if patch.Dose != nil {
		entry.Dose = *patch.Dose
	}
	if patch.YieldAmount != nil {
		entry.YieldAmount = *patch.YieldAmount
	}
	if patch.WaterTemperature != nil {
		entry.WaterTemperature = *patch.WaterTemperature
	}
	if patch.BrewTime != nil {
		entry.BrewTime = *patch.BrewTime
	}
	if patch.BloomTime != nil {
		entry.BloomTime = *patch.BloomTime
	}
	if patch.BloomWater != nil {
		entry.BloomWater = *patch.BloomWater
	}
	if patch.PourNotes != nil {
		entry.PourNotes = *patch.PourNotes
	}
	if patch.RoastLevel != nil {
		entry.RoastLevel = *patch.RoastLevel
	}
	if patch.Notes != nil {
		entry.Notes = *patch.Notes
	}
	if patch.TastingNotes != nil && patch.Notes == nil {
		entry.Notes = *patch.TastingNotes
	}
	if patch.Rating != nil {
		entry.Rating = *patch.Rating
	}
}

func isValidCoffeeEntry(entry *coffeedata.Entry) error {
	requiredFields := []struct {
		name  string
		value string
	}{
		{"date", entry.Date},
		{"coffeeName", entry.CoffeeName},
		{"roasterId", entry.RoasterID},
		{"roaster", entry.Roaster},
		{"brewMethod", entry.BrewMethod},
		{"ratio", entry.Ratio},
		{"grinderId", entry.GrinderID},
		{"grinder", entry.Grinder},
		{"notes", entry.Notes},
	}
	for _, f := range requiredFields {
		if f.value == "" {
			return fmt.Errorf("missing required field %q", f.name)
		}
	}

	if _, err := time.Parse("2006-01-02", entry.Date); err != nil {
		return fmt.Errorf("invalid date %q", entry.Date)
	}

	if entry.DaysSinceRoast < 0 {
		return fmt.Errorf("daysSinceRoast must be non-negative, got %d", entry.DaysSinceRoast)
	}

	if entry.GrindSetting < 0 {
		return fmt.Errorf("grindSetting must be non-negative, got %.1f", entry.GrindSetting)
	}

	if entry.Rating < 0 || entry.Rating > 5 {
		return fmt.Errorf("rating must be 0-5, got %d", entry.Rating)
	}

	return nil
}
