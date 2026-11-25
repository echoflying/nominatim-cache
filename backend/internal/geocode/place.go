package geocode

import (
	"encoding/json"
	"math"
	"math/big"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf16"
)

type Point struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

func (p Point) Valid() bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180
}

func (p Point) Key() string {
	return Fixed4(p.Lat) + "_" + Fixed4(p.Lon)
}

// Fixed4 reproduces JavaScript Number.toFixed(4) on valid coordinates.
func Fixed4(value float64) string {
	scaled := new(big.Rat).SetFloat64(math.Abs(value))
	scaled.Mul(scaled, big.NewRat(10000, 1))
	scaled.Add(scaled, big.NewRat(1, 2))
	rounded := new(big.Int).Quo(scaled.Num(), scaled.Denom())
	result := new(big.Rat).SetFrac(rounded, big.NewInt(10000)).FloatString(4)
	if value < 0 {
		return "-" + result
	}
	return result
}

type Response map[string]any

func (r Response) Text(key string) string {
	s, _ := r[key].(string)
	return s
}

func (r Response) Address() map[string]any {
	if address, ok := r["address"].(map[string]any); ok && address != nil {
		return address
	}
	return map[string]any{}
}

func DecodeResponse(raw []byte) (Response, error) {
	var response Response
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	err := decoder.Decode(&response)
	return response, err
}

type Place struct {
	Name        string `json:"place_name"`
	Level       string `json:"place_level"`
	Quality     string `json:"place_quality"`
	SourceField string `json:"place_source_field"`
	Reason      string `json:"quality_reason,omitempty"`
}

var provincePrefix = regexp.MustCompile(`^(北京|天津|上海|重庆|新疆|西藏|广西|宁夏|内蒙古|黑龙江|吉林|辽宁|河北|山西|江苏|浙江|安徽|福建|江西|山东|河南|湖北|湖南|广东|海南|四川|贵州|云南|陕西|甘肃|青海)`)
var administrativeUnit = regexp.MustCompile(`省|自治区|特别行政区|市|自治州|州|盟|地区|区|县|旗|镇|乡|街道`)

func broadAdministrativePath(value string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == ',' || r == '，' || r == '、' || r == '\ufeff' {
			return -1
		}
		return r
	}, value)
	return provincePrefix.MatchString(normalized) &&
		len(utf16.Encode([]rune(normalized))) >= 18 &&
		len(administrativeUnit.FindAllStringIndex(normalized, 3)) == 3
}

var addressFields = []struct{ field, level string }{
	{"name", "regional"}, {"road", "street"}, {"street", "street"},
	{"pedestrian", "street"}, {"path", "street"}, {"neighbourhood", "neighbourhood"},
	{"suburb", "neighbourhood"}, {"town", "locality"}, {"village", "locality"},
	{"district", "district"}, {"county", "district"}, {"city", "city"},
	{"municipality", "city"}, {"state", "province"},
}

func SelectPlace(r Response) Place {
	category := r.Text("class")
	if r["class"] == nil {
		category = r.Text("category")
	}
	name := strings.TrimSpace(r.Text("name"))
	if name != "" {
		switch category {
		case "amenity", "shop", "tourism", "office", "leisure":
			return Place{Name: name, Level: "poi", Quality: "precise", SourceField: "name"}
		case "building":
			if r.Text("type") != "yes" {
				return Place{Name: name, Level: "poi", Quality: "precise", SourceField: "name"}
			}
		}
	}

	address := r.Address()
	rejected := ""
	for _, item := range addressFields {
		value, _ := address[item.field].(string)
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if item.field == "state" || broadAdministrativePath(value) {
			if rejected == "" {
				rejected = item.field
			}
			continue
		}
		quality := "precise"
		if item.level == "city" || item.level == "regional" {
			quality = "regional"
		}
		return Place{Name: value, Level: item.level, Quality: quality, SourceField: "address." + item.field}
	}
	label, _, _ := strings.Cut(r.Text("display_name"), ",")
	if label = strings.TrimSpace(label); label != "" && rejected == "" && !broadAdministrativePath(label) {
		return Place{Name: label, Level: "label", Quality: "regional", SourceField: "display_name"}
	}
	if rejected != "" {
		return Place{Level: "province", Quality: "coarse", SourceField: "address." + rejected,
			Reason: "只有省级地址或包含多层行政区划的长地址"}
	}
	return Place{Level: "unknown", Quality: "none", SourceField: "none"}
}

func WithPlace(r Response) Response {
	place := SelectPlace(r)
	r["place_name"] = place.Name
	r["place_level"] = place.Level
	r["place_quality"] = place.Quality
	r["place_source_field"] = place.SourceField
	delete(r, "quality_reason")
	if place.Reason != "" {
		r["quality_reason"] = place.Reason
	}
	return r
}

func PlaceType(r Response) string {
	a := r.Address()
	text := Response(a).Text
	switch {
	case text("tourism") == "temple" || text("amenity") == "place_of_worship":
		return "temple"
	case text("natural") == "peak" || text("natural") == "volcano":
		return "peak"
	case text("road") != "" || text("pedestrian") != "":
		return "street"
	case text("city") != "" || text("town") != "" || text("village") != "":
		return "city"
	default:
		return "other"
	}
}
