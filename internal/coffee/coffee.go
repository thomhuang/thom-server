package coffee

import (
	"database/sql"

	"thom-server/internal/data"
)

type Entry struct {
	ID               string  `json:"id"`
	Date             string  `json:"date"`
	CoffeeName       string  `json:"coffeeName"`
	Origin           string  `json:"origin"`
	CoffeeVarietal   string  `json:"coffeeVarietal"`
	ProcessingMethod string  `json:"processingMethod"`
	DaysSinceRoast   int     `json:"daysSinceRoast"`
	RoasterID        string  `json:"roasterId"`
	Roaster          string  `json:"roaster"`
	BrewMethod       string  `json:"brewMethod"`
	Ratio            string  `json:"ratio"`
	GrinderID        string  `json:"grinderId"`
	Grinder          string  `json:"grinder"`
	GrindSetting     float64 `json:"grindSetting"`
	Dose             int     `json:"dose"`
	YieldAmount      int     `json:"yieldAmount"`
	WaterTemperature int     `json:"waterTemperature"`
	BrewTime         string  `json:"brewTime"`
	BloomTime        string  `json:"bloomTime"`
	BloomWater       int     `json:"bloomWater"`
	PourNotes        string  `json:"pourNotes"`
	RoastLevel       string  `json:"roastLevel"`
	Notes            string  `json:"notes"`
	TastingNotes     string  `json:"tastingNotes,omitempty"`
	Rating           int     `json:"rating"`
	CreatedAt        string  `json:"createdAt,omitempty"`
}

type Roaster struct {
	ID        string `json:"id"`
	Roaster   string `json:"roaster"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type Grinder struct {
	ID        string `json:"id"`
	Grinder   string `json:"grinder"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type Model struct {
	DB *sql.DB
}

type scanner interface {
	Scan(dest ...any) error
}

type querier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// SlugifyName turns a display name into the stable id used by the roaster and
// grinder lookups: lowercase alphanumerics separated by single dashes.
func SlugifyName(value string) string {
	return data.SlugifyName(value)
}
