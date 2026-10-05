package rpg

// Passive prose describes executable v3 rules. Conditions absent from the battle
// engine (hero debuffs, accuracy stats, speed) are expressed using supported rules.
var passiveDescriptions = map[string]string{
	"char_001": "Setelah HP benar-benar pulih, serangan berikutnya mendapat bonus 15% (tidak menumpuk).",
	"char_002": "Saat HP pengguna di bawah 50%, perisai yang diberikan bertambah 20%.",
	"char_003": "Damage pada sasaran bertanda bertambah 15% sebelum Tanda dikonsumsi.",
	"char_004": "Menerima 20% lebih sedikit damage dari serangan kuat musuh.",
	"char_005": "Pada awal battle, seluruh tim mendapat perisai 5% HP maksimum.",
	"char_006": "Jika serangan musuh meleset atau dihindari, hit berikutnya mendapat bonus 20% (tidak menumpuk).",
	"char_007": "Saat HP di bawah 50%, pemulihan yang diterima bertambah 20%.",
	"char_008": "Serangan pertama yang mengenai pengguna mendapat pengurangan damage 35%.",
	"char_009": "Menyerang sasaran dengan DEF turun memberi 1 energi, maksimum sekali per ronde.",
	"char_010": "Pada awal battle, seluruh tim mendapat perisai 5% HP maksimum.",
	"char_011": "Pemulihan pada sekutu di bawah 50% HP bertambah 15%.",
	"char_012": "Setelah skill memberi Bara, mendapat 1 energi tambahan, maksimum sekali per ronde.",
	"char_013": "Hit pertama dalam battle mendapat bonus damage 20%.",
	"char_014": "Saat berperisai, damage yang diterima berkurang 15% sebelum perisai menyerapnya.",
	"char_015": "Hit pertama dalam battle memberi 1 energi tambahan.",
	"char_016": "Kabut dari skill meningkatkan peluang musuh meleset menjadi 50%.",
	"char_017": "Pemulihan pertama pada sekutu di bawah 30% HP mendapat bonus 30%, sekali per battle.",
	"char_018": "Menghindari serangan kuat pertama yang diarahkan kepadanya, sekali per battle.",
	"char_019": "Saat skill memecahkan perisai musuh, diri mendapat perisai 10% HP.",
	"char_020": "Setelah skill, diri memulihkan 5% HP maksimum tambahan.",
	"char_021": "Damage terhadap sasaran berkabut bertambah 15%.",
	"char_022": "Setelah skill memberi Bara, mendapat 1 energi, maksimum sekali per ronde.",
	"char_023": "Pemulihan berlebih berubah menjadi perisai, maksimum 25% HP sasaran; tidak menumpuk.",
	"char_024": "Setelah skill pertama, diri mendapat perisai 10% HP, sekali per battle.",
	"char_025": "Saat perisai dari sekutu bertambah, mendapat 1 energi, maksimum sekali per ronde.",
	"char_026": "Saat HP di bawah 50%, damage yang diterima berkurang 20%.",
	"char_027": "Damage terhadap sasaran dengan Bara bertambah 15%.",
	"char_028": "Skill yang mengenai minimal dua musuh memberi 1 energi tambahan, sekali per ronde.",
	"char_029": "Skill pemulihan juga memberi regenerasi 12% HP pada awal ronde berikutnya.",
	"char_030": "Skill pada sasaran juga menurunkan ATK selama 2 fase musuh.",
	"char_031": "Saat perisai diri pecah, hit berikutnya mendapat bonus damage 20%.",
	"char_032": "Perisai pertama yang diberikan lebih kuat 25%, sekali per battle.",
	"char_033": "Saat skill memecahkan perisai musuh, diri mendapat perisai 10% HP.",
	"char_034": "Setelah skill pertama, diri mendapat perisai 15% HP, sekali per battle.",
	"char_035": "Pemulihan pada sekutu berperisai bertambah 20%.",
	"char_036": "Damage terhadap musuh yang bersiap menyerang kuat bertambah 15%.",
	"char_037": "Setelah serangan biasa atau bertahan, hit berikutnya mendapat bonus 10%; tidak menumpuk.",
	"char_038": "Jika perisai buatannya pecah karena serangan, pemiliknya pulih 8% HP, maksimum sekali per ronde per pemilik.",
	"char_039": "Damage terhadap sasaran berperisai bertambah 15%.",
	"char_040": "Serangan biasa memperkuat damage skill berikutnya 15%; tidak menumpuk.",
	"char_041": "Skill pemulihan memberi 1 energi tambahan kepada sasaran, maksimum sekali per ronde.",
	"char_042": "Setelah memakai skill, mendapat 1 energi tambahan, maksimum sekali per ronde.",
	"char_043": "Saat bertahan, damage yang diterima berkurang 10% lagi.",
	"char_044": "Perisai yang diberikan bertambah 15% jika minimal dua anggota tim masih hidup.",
	"char_045": "Menyerang sasaran Lambat memberi 1 energi, maksimum sekali per ronde.",
	"char_046": "Tanda dari skill memperkuat hit berikutnya 40%, menggantikan bonus dasar 25%.",
	"char_047": "Pemulihan pada sekutu di bawah 50% HP bertambah 20%.",
	"char_048": "Saat Bara buatannya melukai musuh, mendapat 1 energi, maksimum sekali per ronde.",
	"char_049": "Saat HP di bawah 50%, damage yang diterima berkurang 15%.",
	"char_050": "Jika perisai buatannya pecah karena serangan, pemiliknya mendapat 1 energi, maksimum sekali per ronde per pemilik.",
	"char_051": "Damage bertambah 10% jika sasaran memiliki minimal dua efek lemah.",
	"char_052": "Bara dari skill juga mengurangi DEF sasaran 10% selama Bara aktif.",
	"char_053": "Setelah memakai skill, mendapat 1 energi tambahan, maksimum sekali per ronde.",
	"char_054": "Setelah skill, memberi perisai 8% HP kepada sekutu hidup dengan energi terendah.",
	"char_055": "Saat menerima tambahan perisai dari sekutu, hit berikutnya mendapat bonus 15%.",
	"char_056": "Memulai battle dengan DEF bertambah 10%.",
	"char_057": "Skill pertama yang memperpanjang Bara memberi 1 ronde tambahan (maksimum 4), sekali per battle.",
	"char_058": "Setelah skill, sekutu hidup dengan energi terendah mendapat 1 energi tambahan, sekali per ronde.",
	"char_059": "Pemulihan pertama pada sekutu di bawah 30% HP memberi perisai 20% HP, sekali per battle.",
	"char_060": "Menyerang sasaran bertanda memberi diri perisai 10% HP, maksimum sekali per ronde.",
}

func passivesEnabled(b *Battle) bool { return b.Rules == RulesVersion }
func passiveEnergy(b *Battle, h *Hero) {
	if h.PassiveRound != b.Round {
		h.Energy = min(5, h.Energy+1)
		h.PassiveRound = b.Round
	}
}
func startPassives(b *Battle) {
	if !passivesEnabled(b) {
		return
	}
	for i := range b.Heroes {
		h := &b.Heroes[i]
		switch h.ID {
		case "char_005", "char_010":
			for j := range b.Heroes {
				giveShield(b, h, &b.Heroes[j], .05)
			}
		case "char_056":
			h.Defense = rounded(float64(h.Defense) * 1.1)
		}
	}
}
func giveShield(b *Battle, source, target *Hero, fraction float64) {
	if !passivesEnabled(b) {
		shieldHero(target, fraction)
		return
	}
	switch source.ID {
	case "char_002":
		if source.HP*2 < source.Max {
			fraction *= 1.2
		}
	case "char_032":
		if !source.PassiveUsed {
			fraction *= 1.25
			source.PassiveUsed = true
		}
	case "char_044":
		if len(weakestHeroes(b)) >= 2 {
			fraction *= 1.15
		}
	}
	setPassiveShield(b, source, target, rounded(float64(target.Max)*fraction))
}
func setPassiveShield(b *Battle, source, target *Hero, amount int) {
	amount = min(target.Max/2, amount)
	if target.HP <= 0 || amount <= target.Shield {
		return
	}
	target.Shield = amount
	target.ShieldSource = source.ID
	if source.ID != target.ID {
		if target.ID == "char_025" {
			passiveEnergy(b, target)
		}
		if target.ID == "char_055" {
			target.PassiveBoost = max(target.PassiveBoost, .15)
		}
	}
}
func healHero(b *Battle, source, target *Hero, amount int, skill bool) {
	if target.HP <= 0 {
		return
	}
	if !passivesEnabled(b) {
		target.HP = min(target.Max, target.HP+amount)
		return
	}
	critical := target.HP*10 < target.Max*3
	if source != nil {
		switch source.ID {
		case "char_011":
			if target.HP*2 < target.Max {
				amount = rounded(float64(amount) * 1.15)
			}
		case "char_017":
			if critical && !source.PassiveUsed {
				amount = rounded(float64(amount) * 1.3)
				source.PassiveUsed = true
			}
		case "char_035":
			if target.Shield > 0 {
				amount = rounded(float64(amount) * 1.2)
			}
		case "char_047":
			if target.HP*2 < target.Max {
				amount = rounded(float64(amount) * 1.2)
			}
		}
	}
	if target.ID == "char_007" && target.HP*2 < target.Max {
		amount = rounded(float64(amount) * 1.2)
	}
	healed := min(target.Max-target.HP, amount)
	target.HP += healed
	if healed > 0 && target.ID == "char_001" {
		target.PassiveBoost = max(target.PassiveBoost, .15)
	}
	if source == nil {
		return
	}
	switch source.ID {
	case "char_023":
		if amount > healed {
			setPassiveShield(b, source, target, min(target.Max/4, amount-healed))
		}
	case "char_029":
		if skill {
			target.Regen = max(1, target.Regen)
		}
	case "char_041":
		if skill && source.PassiveRound != b.Round {
			target.Energy = min(5, target.Energy+1)
			source.PassiveRound = b.Round
		}
	case "char_059":
		if critical && !source.PassiveUsed {
			giveShield(b, source, target, .20)
			source.PassiveUsed = true
		}
	}
}
func passivePower(b *Battle, h *Hero, e *Opponent) float64 {
	if !passivesEnabled(b) {
		return 1
	}
	power := 1 + h.PassiveBoost
	h.PassiveBoost = 0
	switch h.ID {
	case "char_003":
		if e.Mark {
			power *= 1.15
		}
	case "char_009":
		if e.Expose > 0 || e.BurnExpose && e.Burn > 0 {
			passiveEnergy(b, h)
		}
	case "char_013":
		if !h.PassiveUsed {
			power *= 1.2
			h.PassiveUsed = true
		}
	case "char_015":
		if !h.PassiveUsed {
			h.Energy = min(5, h.Energy+1)
			h.PassiveUsed = true
		}
	case "char_021":
		if e.Blind > 0 {
			power *= 1.15
		}
	case "char_027":
		if e.Burn > 0 {
			power *= 1.15
		}
	case "char_036":
		if e.Charged {
			power *= 1.15
		}
	case "char_039":
		if e.Shield > 0 {
			power *= 1.15
		}
	case "char_040":
		if h.SkillBoost && h.CurrentAction == "skill" {
			power *= 1.15
		}
	case "char_045":
		if e.Slow > 0 {
			passiveEnergy(b, h)
		}
	case "char_051":
		n := 0
		for _, v := range []int{e.Burn, e.Expose, e.Weaken, e.Slow, e.Blind} {
			if v > 0 {
				n++
			}
		}
		if e.Mark {
			n++
		}
		if n >= 2 {
			power *= 1.1
		}
	case "char_060":
		if e.Mark && h.PassiveRound != b.Round {
			giveShield(b, h, h, .1)
			h.PassiveRound = b.Round
		}
	}
	return power
}
func passiveSkillTarget(b *Battle, h *Hero, e *Opponent, hadShield bool) {
	if !passivesEnabled(b) {
		return
	}
	switch h.ID {
	case "char_016":
		if e.Blind > 0 {
			e.BlindChance = 50
		}
	case "char_019", "char_033":
		if hadShield && e.Shield == 0 {
			giveShield(b, h, h, .1)
		}
	case "char_030":
		e.Weaken = max(2, e.Weaken)
	case "char_048":
		if e.Burn > 0 {
			e.BurnSource = h.ID
		}
	case "char_052":
		if e.Burn > 0 {
			e.BurnExpose = true
		}
	case "char_057":
		if e.Burn > 0 && !h.PassiveUsed {
			e.Burn = min(4, e.Burn+1)
			h.PassiveUsed = true
		}
	}
}
func passiveAfterAction(b *Battle, h *Hero, action string, hitCount int) {
	if !passivesEnabled(b) {
		return
	}
	if h.ID == "char_037" && (action == "attack" || action == "guard") {
		h.PassiveBoost = max(h.PassiveBoost, .1)
	}
	if h.ID == "char_040" {
		if action == "attack" {
			h.SkillBoost = true
		}
		if action == "skill" {
			h.SkillBoost = false
		}
	}
	if action != "skill" {
		return
	}
	switch h.ID {
	case "char_012", "char_022":
		passiveEnergy(b, h)
	case "char_020":
		healHero(b, h, h, rounded(float64(h.Max)*.05), false)
	case "char_024", "char_034":
		if !h.PassiveUsed {
			f := .1
			if h.ID == "char_034" {
				f = .15
			}
			giveShield(b, h, h, f)
			h.PassiveUsed = true
		}
	case "char_028":
		if hitCount >= 2 {
			passiveEnergy(b, h)
		}
	case "char_042", "char_053":
		passiveEnergy(b, h)
	case "char_054", "char_058":
		ids := weakestHeroes(b)
		if len(ids) == 0 {
			return
		}
		target := &b.Heroes[ids[0]]
		for _, i := range ids {
			if b.Heroes[i].Energy < target.Energy {
				target = &b.Heroes[i]
			}
		}
		if h.ID == "char_054" {
			giveShield(b, h, target, .08)
		} else if h.PassiveRound != b.Round {
			target.Energy = min(5, target.Energy+1)
			h.PassiveRound = b.Round
		}
	}
}
func passiveIncoming(b *Battle, h *Hero, e *Opponent, n int) int {
	if !passivesEnabled(b) || n <= 0 {
		return n
	}
	factor := 1.0
	switch h.ID {
	case "char_004":
		if e.Charged {
			factor = .8
		}
	case "char_008":
		if !h.PassiveUsed {
			factor = .65
			h.PassiveUsed = true
		}
	case "char_014":
		if h.Shield > 0 {
			factor = .85
		}
	case "char_018":
		if e.Charged && !h.PassiveUsed {
			h.PassiveUsed = true
			return 0
		}
	case "char_026":
		if h.HP*2 < h.Max {
			factor = .8
		}
	case "char_043":
		if h.Guard {
			factor = .9
		}
	case "char_049":
		if h.HP*2 < h.Max {
			factor = .85
		}
	}
	return rounded(float64(n) * factor)
}
func passiveShieldBroken(b *Battle, h *Hero) {
	if !passivesEnabled(b) || h.HP <= 0 {
		return
	}
	if h.ID == "char_031" {
		h.PassiveBoost = max(h.PassiveBoost, .2)
	}
	if h.ShieldBreakRound == b.Round {
		return
	}
	switch h.ShieldSource {
	case "char_038":
		healHero(b, nil, h, rounded(float64(h.Max)*.08), false)
		h.ShieldBreakRound = b.Round
	case "char_050":
		h.Energy = min(5, h.Energy+1)
		h.ShieldBreakRound = b.Round
	}
}
