package coffee

import (
	"database/sql"
	"strings"
	"sync"

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

	lookupMu sync.Mutex
	roasters map[string]*Roaster
	grinders map[string]*Grinder
}

// cachedRoaster returns a roaster already resolved this process. The lookup
// tables are only ever appended to (there is no rename or delete route), so a
// cached row stays valid for the life of the process. Every lookup served from
// here is one fewer D1 round trip on a write.
func (m *Model) cachedRoaster(name string) (*Roaster, bool) {
	m.lookupMu.Lock()
	defer m.lookupMu.Unlock()

	roaster, ok := m.roasters[strings.ToLower(name)]
	return roaster, ok
}

func (m *Model) storeRoaster(name string, roaster *Roaster) {
	m.lookupMu.Lock()
	defer m.lookupMu.Unlock()

	if m.roasters == nil {
		m.roasters = make(map[string]*Roaster)
	}
	m.roasters[strings.ToLower(name)] = roaster
}

func (m *Model) cachedGrinder(name string) (*Grinder, bool) {
	m.lookupMu.Lock()
	defer m.lookupMu.Unlock()

	grinder, ok := m.grinders[strings.ToLower(name)]
	return grinder, ok
}

func (m *Model) storeGrinder(name string, grinder *Grinder) {
	m.lookupMu.Lock()
	defer m.lookupMu.Unlock()

	if m.grinders == nil {
		m.grinders = make(map[string]*Grinder)
	}
	m.grinders[strings.ToLower(name)] = grinder
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
