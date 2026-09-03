// Package formfields holds the Google Form "entry.<id>" field mapping for
// the IP PRESCRIPTION AUDIT-MEDICOVER 2026 form.
package formfields

import "strings"

// Canonical lists the Google Form "entry.<id>" field names in the exact
// order the corresponding questions appear on the form / in the spreadsheet.
// This order was reverse engineered from the form's own field metadata
// (FB_PUBLIC_LOAD_DATA_) together with a sample prefilled submission, so it
// lines up positionally with the "real" (non-blank, non-"NA") header columns
// of each monthly sheet in the audit workbook.
//
// Sheets for JANUARY/FEBRUARY omit the two "generic names" questions (indices
// 27 and 28 below); every other month includes them. See WithoutGenericNames.
var Canonical = []string{
	"entry.1388635514", // 0  UHID/IP Number
	"entry.16127784",   // 1  Doctor Name
	"entry.2039185993", // 2  Department
	"entry.591659533",  // 3  Audit Date
	"entry.937275221",  // 4  Location
	"entry.1784678943", // 5  Drug Allergies Documented
	"entry.209346204",  // 6  Total number of drugs in the prescription
	"entry.1751052506", // 7  Were all drugs Doses stated appropriately
	"entry.1561041470", // 8  How many drugs did not have doses stated appropriately
	"entry.1202982505", // 9  Were all drugs Frequency stated appropriately
	"entry.1354743444", // 10 How many drugs did not have Frequency stated appropriately
	"entry.1135952575", // 11 Were all drugs Routes stated appropriately
	"entry.1751732405", // 12 How many drugs did no have Route stated approriately
	"entry.1063818880", // 13 Were all drugs are having units mentioned appropriately
	"entry.1848256539", // 14 Howmany drugs not mentioned Units
	"entry.1376144118", // 15 Were all drugs are having Concentration mentioned
	"entry.1856700929", // 16 Howmany drugs not mentioned Concentration
	"entry.2061050192", // 17 Were all drugs having rate of administration mentioned
	"entry.2056724869", // 18 How many drugs did not have rate of administration mentioned
	"entry.1275427364", // 19 Were all drugs selection was appropriate
	"entry.1495692158", // 20 Howmany drugs were selected inappropriately
	"entry.836345919",  // 21 Was the prescription legible
	"entry.854824962",  // 22 How many drugs are illegible
	"entry.1410566836", // 23 Were only standard abbreviations used in the prescripton
	"entry.1435594645", // 24 How many Non approved abbreviations used
	"entry.1168270256", // 25 Was the prescription written in capital letters
	"entry.956893254",  // 26 How many drug names were not written in capital letters
	"entry.1296653463", // 27 Were all the drugs written in generic names (absent in JAN/FEB)
	"entry.957450069",  // 28 How many drugs are not written in generic names (absent in JAN/FEB)
	"entry.1140705989", // 29 Were Drug-Drug Interactions mentioned appropriately
	"entry.1922209467", // 30 How many drugs had non modification of drug dose (drug-drug)
	"entry.1835851095", // 31 Were Drug-Food interactions mentioned appropriately
	"entry.1426221511", // 32 How many drugs had non modification of time/dose (drug-food)
	"entry.833002773",  // 33 Wrong Formulation Transcribed/Indented
	"entry.1857844228", // 34 Howmany wrong Formulation Transcribed/Indented
	"entry.679578146",  // 35 Wrong Drug Transcribed/Indented
	"entry.1305461127", // 36 Howmany wrong Drug Transcribed/Indented
	"entry.1152469130", // 37 Wrong Strength Transcribed/Indented
	"entry.1672349750", // 38 Howmany wrong Strength Transcribed/Indented
	"entry.2095581020", // 39 Were all drugs dispensed correctly
	"entry.596594851",  // 40 How many wrong drugs dispensed
	"entry.995230279",  // 41 Were all drug doses dispensed correctly
	"entry.85682471",   // 42 Howmany wrong dose dispensed
	"entry.2142693570", // 43 Were all drug formulations dispensed correctly
	"entry.1614786540", // 44 How many wrong drug formulations dispensed
	"entry.489668709",  // 45 Were Drugs dispensed before Expiry
	"entry.1533117056", // 46 How many Expired drugs dispensed
	"entry.582121765",  // 47 Were drugs dispensed with correct labelling
	"entry.685629190",  // 48 How many drugs dispensed in wrong /No drug labelling
	"entry.112244978",  // 49 Were all drugs dispensed with in defined time
	"entry.280016434",  // 50 Howmany drugs were not dispensed in defined time
	"entry.1797529441", // 51 Generic Substitute done without consultation
	"entry.527453480",  // 52 Howmany Generic Substitute done without consultation
	"entry.579723155",  // 53 Were drugs administered to the correct patient
	"entry.2080271879", // 54 How many drugs were administered to wrong patient
	"entry.539645912",  // 55 Were all drugs administered to the patient
	"entry.603507081",  // 56 Howmany drugs were omitted to the patient
	"entry.1511757628", // 57 Were all drugs administered in Correct dose
	"entry.1707724783", // 58 How many drug doses were administered improperly
	"entry.752051084",  // 59 Were all drugs administered correctly
	"entry.235949186",  // 60 How many wrong drugs were adminstered
	"entry.773238462",  // 61 Were all drugs administered in correct dosage form
	"entry.1528301108", // 62 Howmany wrong dosage form administered
	"entry.2098047402", // 63 Were all drugs administered in right route
	"entry.1463831250", // 64 How many wrong route of drugs administered
	"entry.2100423117", // 65 Were all drugs administered in Correct Rate
	"entry.469944753",  // 66 How many drugs were administered in wrong Rate
	"entry.500853983",  // 67 Were all drugs administered in Correct Duration
	"entry.791033352",  // 68 How many drugs were administered in wrong Duration
	"entry.1355742435", // 69 Were all drugs administered in correct time
	"entry.254440007",  // 70 How many drugs were administered in wrong time
	"entry.1827676895", // 71 Was documentation of drug administration done properly
	"entry.1789475478", // 72 How many drugs were not documented the administration
	"entry.2033731235", // 73 Was documentation of drugs completely & properly done by nursing staff
	"entry.1154643070", // 74 How many drugs were documented Incompletly/Improperly
	"entry.1122021611", // 75 Documentation without administration
	"entry.59391624",   // 76 Howmany drugs were documented without administration
	"entry.1683552119", // 77 Audit Observations
}

// genericNamesIndices are the positions within Canonical that are absent
// from sheets that don't ask about generic drug names (JANUARY/FEBRUARY).
var genericNamesIndices = []int{27, 28}

// WithoutGenericNames is Canonical with the generic names question/answer
// pair removed, for sheets with 76 real columns.
func WithoutGenericNames() []string {
	out := make([]string, 0, len(Canonical)-len(genericNamesIndices))
	skip := make(map[int]bool, len(genericNamesIndices))
	for _, i := range genericNamesIndices {
		skip[i] = true
	}
	for i, e := range Canonical {
		if skip[i] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// GenericNamesFieldCount is the number of fields present in Canonical but
// absent from WithoutGenericNames().
func GenericNamesFieldCount() int {
	return len(genericNamesIndices)
}

// AuditDateEntry is the entry field that renders as a native Google Forms
// date picker and therefore requires a strict "2006-01-02" value.
const AuditDateEntry = "entry.591659533"

// AuditObservationsEntry is the free-text "Audit Observations" field. Real
// browser submissions send it as a plain classic param alongside
// partialResponse rather than inside it.
const AuditObservationsEntry = "entry.1683552119"

// choices lists the exact, case-sensitive option text Google Forms expects
// for each single-choice (radio button) question, keyed by entry field name.
// Values in the spreadsheet use inconsistent casing (Yes/YES/yes, No/NO/no)
// so submissions must be normalized to one of these exact strings or the
// form rejects the whole response with a generic "Something went wrong".
var choices = map[string][]string{
	"entry.1784678943": {"Yes", "No"},     // Drug Allergies Documented
	"entry.1751052506": {"YES", "NO"},     // Doses stated appropriately
	"entry.1202982505": {"YES", "NO"},     // Frequency stated appropriately
	"entry.1135952575": {"YES", "NO"},     // Routes stated appropriately
	"entry.1063818880": {"Yes", "No"},     // Units mentioned appropriately
	"entry.1376144118": {"Yes", "No"},     // Concentration mentioned
	"entry.2061050192": {"YES", "NO"},     // Rate of administration mentioned
	"entry.1275427364": {"Yes", "No"},     // Drug selection appropriate
	"entry.836345919":  {"Yes", "No"},     // Prescription legible
	"entry.1410566836": {"Yes", "No"},     // Standard abbreviations used
	"entry.1168270256": {"Yes", "No"},     // Written in capital letters
	"entry.1296653463": {"Yes", "No"},     // Written in generic names
	"entry.1140705989": {"Yes", "No"},     // Drug-Drug interactions mentioned
	"entry.1835851095": {"Yes", "No"},     // Drug-Food interactions mentioned
	"entry.833002773":  {"Yes", "No"},     // Wrong formulation transcribed
	"entry.679578146":  {"Yes", "No"},     // Wrong drug transcribed
	"entry.1152469130": {"Yes", "No"},     // Wrong strength transcribed
	"entry.2095581020": {"YES", "NO"},     // Dispensed correctly
	"entry.995230279":  {"Yes", "No"},     // Doses dispensed correctly
	"entry.2142693570": {"Yes", "No"},     // Formulations dispensed correctly
	"entry.489668709":  {"Yes", "No", ""}, // Dispensed before Expiry
	"entry.582121765":  {"Yes", "No"},     // Correct labelling
	"entry.112244978":  {"Yes", "No"},     // Dispensed in defined time
	"entry.1797529441": {"Yes", "No"},     // Generic substitute without consultation
	"entry.579723155":  {"Yes", "No"},     // Administered to correct patient
	"entry.539645912":  {"Yes", "No"},     // Administered to the patient
	"entry.1511757628": {"Yes", "No"},     // Administered in correct dose
	"entry.752051084":  {"Yes", "No"},     // Administered correctly
	"entry.773238462":  {"Yes", "No"},     // Administered in correct dosage form
	"entry.2098047402": {"Yes", "No"},     // Administered in right route
	"entry.2100423117": {"Yes", "No"},     // Administered in Correct Rate
	"entry.500853983":  {"Yes", "No"},     // Administered in Correct Duration
	"entry.1355742435": {"Yes", "No"},     // Administered in correct time
	"entry.1827676895": {"Yes", "No"},     // Documentation done properly
	"entry.2033731235": {"Yes", "No"},     // Documentation by nursing staff
	"entry.1122021611": {"Yes", "No"},     // Documentation without administration
}

// fuzzyEditDistance is the maximum Levenshtein distance allowed when
// falling back to a nearest-match guess (e.g. "N0" -> "No", "NOO" -> "No").
const fuzzyEditDistance = 1

// NormalizeChoice matches raw against the accepted option text for entry,
// returning the exact casing Google Forms requires. It first tries a
// case-insensitive exact match, then falls back to the nearest option within
// fuzzyEditDistance edits (only when exactly one option is that close). If
// entry has no known choice set, raw is returned as-is. If nothing matches,
// raw is returned unchanged with ok=false and fuzzy=false.
func NormalizeChoice(entry, raw string) (value string, ok bool, fuzzy bool) {
	options, known := choices[entry]
	if !known {
		return raw, true, false
	}
	for _, opt := range options {
		if strings.EqualFold(opt, raw) {
			return opt, true, false
		}
	}

	best := ""
	bestDist := -1
	ambiguous := false
	rawLower := strings.ToLower(raw)
	for _, opt := range options {
		d := levenshtein(rawLower, strings.ToLower(opt))
		if bestDist == -1 || d < bestDist {
			bestDist, best, ambiguous = d, opt, false
		} else if d == bestDist {
			ambiguous = true
		}
	}
	if bestDist >= 0 && bestDist <= fuzzyEditDistance && !ambiguous {
		return best, true, true
	}
	return raw, false, false
}

// levenshtein returns the edit distance between a and b.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

// IsChoice reports whether entry is a radio-button field that uses Google's
// companion sentinel input in a browser submission.
func IsChoice(entry string) bool {
	_, ok := choices[entry]
	return ok
}
