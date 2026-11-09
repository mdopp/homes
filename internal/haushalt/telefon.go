package haushalt

import (
	"cmp"
	"context"
	"fmt"
	"strings"

	"github.com/mdopp/homes/internal/caldav"
)

// vorwahlen are the German area codes (without the 0) Telefon splits off as they are, two and
// three digits, and the service numbers; another area outside 03… has four, one in 03… is not split.
const vorwahlen = " 30 40 69 89 201 202 203 208 209 211 212 214 221 228 231 234 241 251 261 271 281 291 " +
	"331 335 340 341 345 351 355 361 365 371 375 381 385 391 395 421 431 441 451 461 471 481 491 511 521 531 541 551 561 571 581 591 " +
	"611 621 631 641 651 661 671 681 711 721 731 741 751 761 771 781 791 821 831 841 851 861 871 881 906 911 921 931 941 951 961 971 981 991 " +
	"700 800 900 1801 1802 1803 1804 1805 1806 1807 "

// Telefon is a number in the household's form (DIN 5008, owner
// 2026-11-09): „+49 541 564890“, the area code apart where Homes knows it
// for sure (mobile 15x–17x, service, the table above, four digits for the
// other areas outside 03…), else „+49 <the rest>“. ok false: not a German
// number Homes reads for sure (another country, no area code, a word in
// it, a length no such number has); then it is s as written.
func Telefon(s, cc string) (string, bool) {
	v := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "tel:"))
	if !phoneRE.MatchString(v) || !strings.HasPrefix(v, "+") && !strings.HasPrefix(digits(v), "0") {
		return s, false
	}
	n, ok := strings.CutPrefix(International(v, cc), "49")
	if !ok || len(n) < 6 || len(n) > 13 {
		return s, false
	}
	area, short, long := 0, 3, 13 // the digits after the area code
	for l := 2; l <= 4 && area == 0; l++ {
		if strings.Contains(vorwahlen, " "+n[:l]+" ") {
			area = l
		}
	}
	switch {
	case n[0] == '1' && strings.ContainsRune("567", rune(n[1])): // mobile: 10 or 11 digits
		area, short, long = 3, 7, 8
	case area > 0 && (n[0] == '1' || n[1:3] == "00"): // service: 0180x, 0700, 0800, 0900
		short, long = 6, 8
	case area == 0 && n[0] != '1' && n[0] != '3':
		area = 4
	}
	if r := len(n) - area; r < short || r > long {
		return s, false
	}
	if i := strings.LastIndex(v, "-"); area > 0 && i > 0 && strings.Trim(v[i+1:], "0123456789 ") == "" { // DIN 5008 keeps an extension apart: „+49 681 844-0“
		if k := len(digits(v[i+1:])); k <= 5 && len(n)-area-k >= 2 {
			return "+49 " + n[:area] + " " + n[area:len(n)-k] + "-" + n[len(n)-k:], true
		}
	}
	return "+49 " + strings.TrimLeft(n[:area]+" "+n[area:], " "), true
}

// vereinheitlichen puts every number of the household's cards in
// the household's form (owner 2026-11-09), also on cards edited by hand:
// they are the household's, not a person's own book. Without apply it only
// says what it would change. A number Telefon cannot read for sure stays
// and is listed as unklar. Each card is written line by line over the
// version read (CorrectShared): a line's TYPE stays.
func vereinheitlichen(ctx context.Context, c *caldav.Client, cc string, a map[string]any) (string, error) {
	apply, _ := a["apply"].(bool)
	all, partial, err := c.Cards(ctx)
	if err != nil {
		return "", err
	}
	var neu, unklar, fehler []string
	for _, cd := range all {
		name := cut(cmp.Or(oneLine(cd.Name), oneLine(cd.Org), "(ohne Namen)"), 100)
		ks, from := []caldav.Korrektur(nil), len(neu)
		for _, p := range cd.Phones {
			t, ok := Telefon(p, cc)
			switch {
			case oneLine(p) == "":
			case !ok:
				unklar = append(unklar, name+": "+oneLine(p))
			case t != p:
				ks, neu = append(ks, caldav.Korrektur{Prop: "TEL", Alt: p, Neu: t}), append(neu, name+": "+oneLine(p)+" → "+t)
			}
		}
		if apply && len(ks) > 0 {
			if _, err := c.CorrectShared(ctx, oneLine(cd.UID), ks); err != nil {
				fehler, neu = append(fehler, name+": "+err.Error()), neu[:from]
			}
		}
	}
	head := fmt.Sprintf("Probelauf: %d Nummern würden vereinheitlicht, %d sind unklar und bleiben.", len(neu), len(unklar))
	if apply {
		head = fmt.Sprintf("%d Nummern vereinheitlicht, %d unklar und geblieben, %d Kontakte nicht geschrieben.", len(neu), len(unklar), len(fehler))
	}
	if partial {
		head += " Nur ein Teil des Adressbuchs ist gelesen (zu viele Kontakte)."
	}
	for i, ls := range [][]string{neu, unklar, fehler} {
		if len(ls) > 0 {
			head += "\n\n" + [...]string{"Neu:", "Unklar:", "Nicht geschrieben:"}[i] + "\n" + strings.Join(ls, "\n")
		}
	}
	return head, nil
}
