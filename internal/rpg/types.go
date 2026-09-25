package rpg

import "fmt"

type Error struct {
	Status  int
	Code    string
	Message string
}

func (e *Error) Error() string                    { return e.Message }
func fail(status int, code, message string) error { return &Error{status, code, message} }
func conflict() error {
	return fail(409, "stale_revision", "Keadaan permainan sudah berubah. Muat ulang untuk melanjutkan.")
}

type Profile struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Revision   int64          `json:"revision"`
	Version    int            `json:"version"`
	Shards     int            `json:"shards"`
	Coins      int            `json:"coins"`
	Dust       int            `json:"dust"`
	Pity5      int            `json:"pity5"`
	Pity4      int            `json:"pity4"`
	Guarantee  bool           `json:"guarantee"`
	Unlocked   int            `json:"unlocked"`
	Cleared    []int          `json:"cleared"`
	Party      []string       `json:"party"`
	Collection map[string]int `json:"collection"`
	LastBattle string         `json:"last_battle"`
}
type Hero struct {
	Character
	HP      int  `json:"hp"`
	Max     int  `json:"max"`
	Attack  int  `json:"attack"`
	Defense int  `json:"defense"`
	Energy  int  `json:"energy"`
	Acted   bool `json:"acted"`
	Guard   bool `json:"guard"`
}
type Opponent struct {
	Enemy
	HP      int    `json:"hp"`
	Max     int    `json:"max"`
	Attack  int    `json:"attack"`
	Defense int    `json:"defense"`
	Kind    string `json:"kind"`
	Charged bool   `json:"charged"`
}
type Battle struct {
	ID           string     `json:"id"`
	Revision     int64      `json:"revision"`
	Rules        string     `json:"rules"`
	Stage        int        `json:"stage"`
	Round        int        `json:"round"`
	Active       int        `json:"active"`
	Target       int        `json:"target"`
	Phase        string     `json:"phase"`
	Done         bool       `json:"done"`
	Result       string     `json:"result"`
	FirstClear   bool       `json:"first_clear"`
	RewardShards int        `json:"reward_shards"`
	RewardCoins  int        `json:"reward_coins"`
	Heroes       []Hero     `json:"heroes"`
	Enemies      []Opponent `json:"enemies"`
	Logs         []string   `json:"logs"`
}

func (b *Battle) log(message string) {
	b.Logs = append([]string{fmt.Sprintf("R%d · %s", b.Round, message)}, b.Logs...)
	if len(b.Logs) > 60 {
		b.Logs = b.Logs[:60]
	}
}

type Pull struct {
	Character Character `json:"character"`
	Duplicate bool      `json:"duplicate"`
}
type Snapshot struct {
	Profile Profile `json:"profile"`
	Battle  *Battle `json:"battle"`
	Results []Pull  `json:"results,omitempty"`
}
type Action struct {
	RequestID string `json:"request_id"`
	Revision  int64  `json:"expected_revision"`
	Actor     int    `json:"actor"`
	Action    string `json:"action"`
	Target    int    `json:"target"`
}
type PartyRequest struct {
	RequestID string   `json:"request_id"`
	Revision  int64    `json:"expected_revision"`
	Party     []string `json:"party"`
}
