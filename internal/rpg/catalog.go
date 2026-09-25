package rpg

import (
	"embed"
	"encoding/json"
	"fmt"
)

//go:embed catalog/*.json
var catalogFiles embed.FS

const RulesVersion = "arunika-v1"

type Ability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}
type Art struct {
	SpritePrompt   string `json:"sprite_prompt"`
	PortraitPrompt string `json:"portrait_prompt,omitempty"`
}
type Character struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Rarity      int     `json:"rarity"`
	Role        string  `json:"role"`
	Element     string  `json:"element"`
	RegionID    string  `json:"region_id"`
	Visual      string  `json:"visual"`
	Personality string  `json:"personality"`
	Skill       Ability `json:"skill"`
	Passive     Ability `json:"passive"`
	Art         Art     `json:"art"`
}
type Enemy struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	RegionID  string `json:"region_id"`
	Element   string `json:"element"`
	Archetype string `json:"archetype"`
	MinLevel  int    `json:"min_level"`
	MaxLevel  int    `json:"max_level"`
	Visual    string `json:"visual"`
	Ability   string `json:"ability"`
	Art       Art    `json:"art"`
}
type Region struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	MinLevel int      `json:"min_level"`
	MaxLevel int      `json:"max_level"`
	Palette  []string `json:"palette"`
	Lore     string   `json:"lore"`
}
type Stage struct {
	Name   string `json:"name"`
	Level  int    `json:"level"`
	Region int    `json:"region"`
	Icon   string `json:"icon"`
	Label  string `json:"label"`
	Boss   bool   `json:"boss"`
	Elite  bool   `json:"elite"`
}
type Catalog struct {
	Version    string      `json:"version"`
	Characters []Character `json:"characters"`
	Enemies    []Enemy     `json:"enemies"`
	Regions    []Region    `json:"regions"`
	Stages     []Stage     `json:"stages"`
	Featured   string      `json:"featured"`
}

func loadCatalog() (Catalog, error) {
	c := Catalog{Version: RulesVersion, Featured: "char_005"}
	for name, out := range map[string]any{"characters": &c.Characters, "enemies": &c.Enemies, "regions": &c.Regions} {
		raw, err := catalogFiles.ReadFile("catalog/" + name + ".json")
		if err != nil {
			return c, err
		}
		if err = json.Unmarshal(raw, out); err != nil {
			return c, err
		}
	}
	if len(c.Characters) != 60 || len(c.Enemies) != 100 || len(c.Regions) != 10 {
		return c, fmt.Errorf("invalid RPG catalog")
	}
	for level := 1; level <= 999; level++ {
		found := false
		for i, r := range c.Regions {
			if level < r.MinLevel || level > r.MaxLevel {
				continue
			}
			boss := level == r.MaxLevel
			elite := !boss && level%10 == 0
			label, icon := "Jalur cerita", "✧"
			if elite {
				label, icon = "Musuh elite", "♧"
			}
			if boss {
				label, icon = "Penjaga lentera", "♜"
			}
			c.Stages = append(c.Stages, Stage{Name: fmt.Sprintf("%s · %03d", r.Name, level), Level: level, Region: i, Icon: icon, Label: label, Boss: boss, Elite: elite})
			found = true
			break
		}
		if !found {
			return c, fmt.Errorf("missing region at level %d", level)
		}
	}
	return c, nil
}
