package rpg

import (
	"fmt"
	"slices"
	"strings"
)

// The active kit is explicit per character. V3 passives are applied by battle hooks.
// Rounds are player phase + enemy phase; Slow reduces the next enemy attack by 20%.
type skillKit struct {
	Power       float64
	Targets     int // 0: all living enemies, 1: selected, 2: selected plus another
	Effects     string
	Support     string // lowest, self, all, two
	Heal        float64
	Shield      float64
	Bonus       string
	Description string
}

var skillKits = map[string]skillKit{
	"char_001": {Power: 1.65, Targets: 1, Effects: "burn", Description: "Damage 1,65× ke sasaran dan Bara selama 2 ronde."},
	"char_002": {Support: "lowest", Shield: .32, Effects: "taunt", Description: "Perisai 32% HP untuk sekutu paling terluka; Nira menarik serangan pada fase musuh berikutnya."},
	"char_003": {Power: 1.65, Targets: 1, Effects: "mark", Description: "Damage 1,65× dan Tanda: serangan berikutnya ke sasaran mendapat bonus 25%."},
	"char_004": {Power: 1.15, Targets: 0, Effects: "slow", Description: "Damage 1,15× ke semua musuh dan Lambat: damage serangan mereka turun 20% untuk 1 fase."},
	"char_005": {Support: "lowest", Heal: .48, Effects: "energy", Description: "Memulihkan 48% HP maksimum sekutu paling terluka dan memberinya 1 energi."},
	"char_006": {Power: 1.5, Targets: 1, Effects: "blind", Description: "Damage 1,5× dan Kabut: peluang serangan musuh meleset 35% selama 1 fase."},
	"char_007": {Power: 1.8, Targets: 1, Effects: "weaken", Description: "Damage 1,8× dan menurunkan ATK sasaran 20% selama 1 fase."},
	"char_008": {Support: "all", Shield: .18, Description: "Memberi setiap sekutu hidup perisai sebesar 18% HP maksimumnya."},
	"char_009": {Power: 1.75, Targets: 1, Effects: "expose", Description: "Damage 1,75× dan menurunkan DEF sasaran 25% selama 2 fase musuh."},
	"char_010": {Power: 1.35, Targets: 0, Effects: "dispel", Description: "Damage 1,35× ke semua musuh dan membatalkan persiapan serangan kuat mereka."},
	"char_011": {Support: "lowest", Heal: .38, Effects: "guard", Description: "Memulihkan 38% HP sekutu paling terluka dan mengurangi damage yang diterimanya 50% untuk 1 fase."},
	"char_012": {Power: 1.1, Targets: 2, Effects: "burn,energy_self", Description: "Damage 1,1× dan Bara 2 ronde ke dua sasaran; memulihkan 1 energi pengguna."},
	"char_013": {Power: 1.95, Targets: 1, Effects: "slow", Description: "Damage 1,95× ke sasaran dan menurunkan damage serangannya 20% untuk 1 fase."},
	"char_014": {Support: "self", Shield: .48, Effects: "protect", Description: "Perisai diri 48% HP dan melindungi sekutu paling terluka dengan pengurangan damage 50% selama 1 fase."},
	"char_015": {Power: 2.0, Targets: 1, Bonus: "full", Description: "Damage 2×; bonus 35% jika sasaran masih memiliki HP penuh."},
	"char_016": {Power: 1.0, Targets: 0, Effects: "blind", Description: "Damage 1× ke semua musuh dan peluang meleset 35% pada serangan berikutnya."},
	"char_017": {Support: "all", Heal: .12, Effects: "regen", Description: "Memulihkan 12% HP seluruh tim, lalu 12% lagi pada awal 2 ronde berikutnya."},
	"char_018": {Power: 1.8, Targets: 1, Effects: "dispel,shield_self", Description: "Damage 1,8×, membatalkan serangan kuat sasaran, dan memberi diri perisai 12% HP."},
	"char_019": {Power: 2.2, Targets: 1, Bonus: "shield", Effects: "shatter", Description: "Damage 2,2×, bonus 35% pada musuh berperisai; menghancurkan sisa perisainya."},
	"char_020": {Support: "lowest", Shield: .30, Effects: "guard,heal_self", Description: "Perisai 30% HP dan pengurangan damage 50% untuk sekutu paling terluka selama 1 fase; diri pulih 12% HP."},
	"char_021": {Power: 2.1, Targets: 1, Bonus: "blind", Description: "Damage 2,1×; bonus 25% jika sasaran terkena Kabut."},
	"char_022": {Power: 1.2, Targets: 0, Effects: "burn_primary", Description: "Damage 1,2× ke semua musuh; sasaran utama mendapat Bara selama 2 ronde."},
	"char_023": {Support: "lowest", Heal: .38, Shield: .15, Description: "Memulihkan 38% HP sekutu paling terluka dan memberi perisai 15% HP."},
	"char_024": {Power: 1.6, Targets: 1, Effects: "dispel,evade", Description: "Damage 1,6× dan membatalkan serangan kuat sasaran; menghindari satu serangan berikutnya."},
	"char_025": {Power: 2.0, Targets: 1, Effects: "mark", Description: "Damage 2× dan memperkuat serangan berikutnya pada sasaran sebesar 25%."},
	"char_026": {Support: "all", Effects: "guard", Description: "Seluruh sekutu hidup menerima pengurangan damage 50% selama fase musuh berikutnya."},
	"char_027": {Power: 1.8, Targets: 1, Effects: "extend_burn", Bonus: "burn", Description: "Damage 1,8×, bonus 25% pada sasaran terbakar; perpanjang Bara 1 ronde (maksimum 3)."},
	"char_028": {Power: 1.35, Targets: 2, Effects: "slow,energy_self", Description: "Damage 1,35× dan Lambat 1 fase pada dua sasaran; memulihkan 1 energi pengguna."},
	"char_029": {Support: "lowest", Heal: .42, Effects: "energy", Description: "Memulihkan 42% HP sekutu paling terluka dan memberinya 1 energi."},
	"char_030": {Power: 1.85, Targets: 1, Effects: "slow_long,weaken", Description: "Damage 1,85×; Lambat 2 fase dan ATK sasaran turun 20% selama 1 fase."},
	"char_031": {Power: 1.9, Targets: 1, Bonus: "own_shield", Description: "Damage 1,9×; bonus 35% jika pengguna memiliki perisai."},
	"char_032": {Support: "self", Shield: .40, Effects: "reflect", Description: "Perisai diri 40% HP; memantulkan 25% damage dari satu serangan berikutnya."},
	"char_033": {Power: 2.0, Targets: 1, Effects: "shatter", Description: "Damage 2×, lalu menghancurkan sisa perisai sasaran."},
	"char_034": {Power: 1.25, Targets: 0, Effects: "slow_long", Description: "Damage 1,25× ke semua musuh dan Lambat selama 2 fase musuh."},
	"char_035": {Support: "lowest", Heal: .45, Shield: .20, Description: "Memulihkan 45% HP sekutu paling terluka dan memberinya perisai 20% HP."},
	"char_036": {Power: 1.9, Targets: 1, Effects: "dispel", Bonus: "charged", Description: "Damage 1,9×, bonus 25% jika musuh bersiap menyerang kuat; batalkan persiapannya."},
	"char_037": {Power: 1.55, Targets: 1, Effects: "burn,weaken", Description: "Damage 1,55×, Bara 2 ronde, dan ATK sasaran turun 20% selama 1 fase."},
	"char_038": {Support: "two", Shield: .34, Description: "Memberi dua sekutu paling terluka perisai sebesar 34% HP maksimum masing-masing."},
	"char_039": {Power: 2.0, Targets: 1, Bonus: "shield", Effects: "pierce", Description: "Damage 2× yang melewati perisai; bonus 35% pada sasaran berperisai."},
	"char_040": {Power: 1.45, Targets: 0, Effects: "slow_hurt", Description: "Damage 1,45× ke semua musuh; musuh yang sudah terluka mendapat Lambat 1 fase."},
	"char_041": {Support: "lowest", Heal: .35, Shield: .12, Effects: "energy", Description: "Memulihkan 35% HP sekutu paling terluka, perisai 12% HP, dan 1 energi."},
	"char_042": {Power: 1.6, Targets: 1, Effects: "dispel,energy_self", Description: "Damage 1,6×, membatalkan serangan kuat musuh, dan memulihkan 1 energi diri."},
	"char_043": {Power: 1.9, Targets: 1, Effects: "expose_charged,guard_self", Description: "Damage 1,9×; musuh bersiap menyerang kuat mendapat DEF turun 25% untuk 2 fase. Diri bertahan 1 fase."},
	"char_044": {Support: "all", Shield: .24, Description: "Memberi seluruh tim perisai sebesar 24% HP maksimum masing-masing."},
	"char_045": {Power: 2.15, Targets: 1, Effects: "slow", Bonus: "charged", Description: "Damage 2,15× dan Lambat 1 fase; bonus 25% pada sasaran yang bersiap menyerang kuat."},
	"char_046": {Power: 1.15, Targets: 0, Effects: "mark_highest", Description: "Damage 1,15× ke semua musuh; musuh tersisa dengan HP tertinggi mendapat Tanda."},
	"char_047": {Support: "all", Heal: .20, Effects: "guard", Description: "Memulihkan 20% HP seluruh tim dan mengurangi damage yang diterima 50% untuk 1 fase."},
	"char_048": {Power: 1.65, Targets: 1, Effects: "burn,evade", Description: "Damage 1,65× dan Bara 2 ronde; pengguna menghindari satu serangan berikutnya."},
	"char_049": {Power: 2.15, Targets: 1, Bonus: "stronger", Description: "Damage 2,15×; bonus 35% jika persentase HP sasaran lebih tinggi dari pengguna."},
	"char_050": {Support: "lowest", Shield: .38, Effects: "reflect", Description: "Perisai 38% HP untuk sekutu paling terluka; memantulkan 25% damage satu serangan berikutnya."},
	"char_051": {Power: 2.0, Targets: 1, Bonus: "debuffs", Description: "Damage 2×; bonus 40% jika sasaran memiliki setidaknya dua efek lemah."},
	"char_052": {Power: 1.15, Targets: 0, Effects: "burn_unshielded", Description: "Damage 1,15× ke semua musuh; musuh tanpa perisai mendapat Bara 2 ronde."},
	"char_053": {Support: "lowest", Heal: .46, Effects: "energy_self", Description: "Memulihkan 46% HP sekutu paling terluka dan 1 energi pengguna."},
	"char_054": {Power: 1.7, Targets: 1, Effects: "energy_ally", Description: "Damage 1,7× dan memberi 1 energi kepada sekutu dengan energi terendah."},
	"char_055": {Power: 2.1, Targets: 1, Effects: "empower", Description: "Damage 2,1×; memperkuat serangan berikutnya sekutu berenergi terendah sebesar 25%."},
	"char_056": {Support: "lowest", Shield: .15, Effects: "guard,taunt", Description: "Perisai 15% HP dan pengurangan damage 50% untuk sekutu paling terluka; menarik serangan 1 fase."},
	"char_057": {Power: 1.9, Targets: 1, Effects: "boost_burn", Description: "Damage 1,9×; Bara aktif diperkuat 25% (maksimum 12% HP sasaran per ronde) dan diperpanjang 1 ronde."},
	"char_058": {Power: 1.3, Targets: 0, Effects: "weaken_marked,energy_ally", Description: "Damage 1,3× ke semua musuh; musuh bertanda mendapat ATK turun 20% selama 1 fase. Sekutu berenergi terendah mendapat 1 energi."},
	"char_059": {Support: "all", Heal: .28, Effects: "protect", Description: "Memulihkan 28% HP seluruh tim dan mengurangi damage pada sekutu paling terluka 50% selama 1 fase."},
	"char_060": {Power: 2.0, Targets: 1, Effects: "shield_self", Bonus: "marked", Description: "Damage 2×, mengabaikan 50% DEF jika sasaran bertanda; diri mendapat perisai 12% HP."},
}

func damageOpponent(e *Opponent, n int, pierce bool) {
	if !pierce {
		absorbed := min(e.Shield, n)
		e.Shield -= absorbed
		n -= absorbed
	}
	e.HP = max(0, e.HP-n)
}

func (s *Service) heroHit(b *Battle, h *Hero, e *Opponent, power float64, ignoreDefense bool) (int, error) {
	level, def := b.Stage+1, e.Defense
	power *= passivePower(b, h, e)
	if passivesEnabled(b) && e.BurnExpose && e.Burn > 0 {
		def = rounded(float64(def) * .9)
	}
	if modernBattle(b) {
		level = h.Level
		if e.Expose > 0 {
			def = rounded(float64(def) * .75)
		}
		if ignoreDefense {
			def /= 2
		}
		if e.Mark {
			bonus := 25
			if passivesEnabled(b) {
				bonus = max(bonus, e.MarkBonus)
			}
			power *= 1 + float64(bonus)/100
			e.MarkBonus = 0
			e.Mark = false
		}
		if h.Empower {
			power *= 1.25
			h.Empower = false
		}
	}
	return s.hit(h.Attack, def, level, power, h.Element, e.Element, false)
}

func weakestHeroes(b *Battle) []int {
	ids := []int{}
	for i, h := range b.Heroes {
		if h.HP > 0 {
			ids = append(ids, i)
		}
	}
	slices.SortStableFunc(ids, func(a, c int) int { return b.Heroes[a].HP*b.Heroes[c].Max - b.Heroes[c].HP*b.Heroes[a].Max })
	return ids
}

func shieldHero(h *Hero, fraction float64) {
	h.Shield = max(h.Shield, min(h.Max/2, rounded(float64(h.Max)*fraction)))
}

func (s *Service) useSkill(b *Battle, actor, target int) error {
	h := &b.Heroes[actor]
	k, ok := skillKits[h.ID]
	if !ok {
		return fmt.Errorf("missing active kit")
	}
	h.CurrentAction = "skill"
	defer func() { h.CurrentAction = "" }()
	h.Energy -= 2
	hitCount := 0
	effects := strings.Split(k.Effects, ",")
	has := func(v string) bool { return slices.Contains(effects, v) }
	allies := weakestHeroes(b)
	if k.Support != "" {
		selected := allies
		switch k.Support {
		case "self":
			selected = []int{actor}
		case "lowest":
			selected = allies[:1]
		case "two":
			selected = allies[:min(2, len(allies))]
		}
		for _, i := range selected {
			a := &b.Heroes[i]
			if k.Heal > 0 {
				healHero(b, h, a, rounded(float64(a.Max)*k.Heal), true)
			}
			if k.Shield > 0 {
				giveShield(b, h, a, k.Shield)
			}
			if has("guard") {
				a.Guard = true
			}
			if has("regen") {
				a.Regen = 2
			}
			if has("reflect") {
				a.Reflect = true
			}
			if has("energy") {
				a.Energy = min(5, a.Energy+1)
			}
		}
	} else {
		targets := []int{target}
		for i, e := range b.Enemies {
			if e.HP > 0 && i != target && (k.Targets == 0 || len(targets) < k.Targets) {
				targets = append(targets, i)
			}
		}
		for _, i := range targets {
			e := &b.Enemies[i]
			if e.HP <= 0 {
				continue
			}
			hitCount++
			power := k.Power
			marked, charged, hurt, shielded := e.Mark, e.Charged, e.HP < e.Max, e.Shield > 0
			switch k.Bonus {
			case "full":
				if !hurt {
					power *= 1.35
				}
			case "shield":
				if shielded {
					power *= 1.35
				}
			case "own_shield":
				if h.Shield > 0 {
					power *= 1.35
				}
			case "blind":
				if e.Blind > 0 {
					power *= 1.25
				}
			case "burn":
				if e.Burn > 0 {
					power *= 1.25
				}
			case "charged":
				if charged {
					power *= 1.25
				}
			case "stronger":
				if e.HP*h.Max > h.HP*e.Max {
					power *= 1.35
				}
			case "debuffs":
				n := 0
				for _, v := range []int{e.Burn, e.Weaken, e.Expose, e.Blind, e.Slow} {
					if v > 0 {
						n++
					}
				}
				if marked {
					n++
				}
				if n >= 2 {
					power *= 1.4
				}
			}
			n, err := s.heroHit(b, h, e, power, k.Bonus == "marked" && marked)
			if err != nil {
				return err
			}
			damageOpponent(e, n, has("pierce"))
			for _, effect := range effects {
				switch effect {
				case "burn", "burn_primary", "burn_unshielded":
					if effect == "burn_primary" && i != target {
						continue
					}
					if effect == "burn_unshielded" && shielded {
						continue
					}
					if e.Burn == 0 {
						e.BurnExpose = false
						e.BurnSource = ""
					}
					e.Burn = max(e.Burn, 2)
					e.BurnPower = max(e.BurnPower, min(rounded(float64(e.Max)*.04), rounded(float64(h.Attack)*.4)))
				case "extend_burn", "boost_burn":
					if e.Burn > 0 {
						e.Burn = max(e.Burn, min(3, e.Burn+1))
						if effect == "boost_burn" {
							e.BurnPower = min(rounded(float64(e.Max)*.12), rounded(float64(e.BurnPower)*1.25))
						}
					}
				case "weaken":
					e.Weaken = max(1, e.Weaken)
				case "weaken_marked":
					if marked {
						e.Weaken = max(1, e.Weaken)
					}
				case "expose":
					e.Expose = max(2, e.Expose)
				case "expose_charged":
					if charged {
						e.Expose = max(2, e.Expose)
					}
				case "blind":
					if e.Blind == 0 {
						e.BlindChance = 0
					}
					e.Blind = max(1, e.Blind)
				case "slow":
					e.Slow = max(1, e.Slow)
				case "slow_long":
					e.Slow = max(2, e.Slow)
				case "slow_hurt":
					if hurt {
						e.Slow = max(1, e.Slow)
					}
				case "mark":
					e.Mark = true
				case "shatter":
					e.Shield = 0
				case "dispel":
					e.Charged = false
				}
			}
			passiveSkillTarget(b, h, e, shielded)
			b.log(fmt.Sprintf("%s · %s → %s: %d damage.", h.Name, h.Skill.Name, e.Name, n))
		}
	}
	if has("taunt") {
		h.Taunt = true
	}
	if has("evade") {
		h.Evade = true
	}
	if has("guard_self") {
		h.Guard = true
	}
	if has("shield_self") {
		giveShield(b, h, h, .12)
	}
	if has("heal_self") {
		healHero(b, h, h, rounded(float64(h.Max)*.12), false)
	}
	if has("protect") {
		b.Heroes[allies[0]].Guard = true
	}
	if has("energy_self") {
		h.Energy = min(5, h.Energy+1)
	}
	if has("energy_ally") || has("empower") {
		index := allies[0]
		for _, i := range allies {
			if b.Heroes[i].Energy < b.Heroes[index].Energy {
				index = i
			}
		}
		if has("energy_ally") {
			b.Heroes[index].Energy = min(5, b.Heroes[index].Energy+1)
		}
		if has("empower") {
			b.Heroes[index].Empower = true
		}
	}
	if has("mark_highest") {
		index := -1
		for i, e := range b.Enemies {
			if e.HP > 0 && (index < 0 || e.HP > b.Enemies[index].HP) {
				index = i
			}
		}
		if index >= 0 {
			b.Enemies[index].Mark = true
			if passivesEnabled(b) && h.ID == "char_046" {
				b.Enemies[index].MarkBonus = 40
			}
		}
	}
	passiveAfterAction(b, h, "skill", hitCount)
	b.log(h.Name + " memakai " + h.Skill.Name + ".")
	return nil
}
