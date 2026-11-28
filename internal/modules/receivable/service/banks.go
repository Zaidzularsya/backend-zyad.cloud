package service

// Bank: bank tujuan setoran. Code dipakai sebagai nilai tersimpan; Name ditampilkan.
type Bank struct{ Code, Name string }

var banks = []Bank{
	{"BCA", "Bank Central Asia (BCA)"},
	{"BRI", "Bank Rakyat Indonesia (BRI)"},
	{"BNI", "Bank Negara Indonesia (BNI)"},
	{"MANDIRI", "Bank Mandiri"},
	{"BSI", "Bank Syariah Indonesia (BSI)"},
	{"CIMB", "CIMB Niaga"},
	{"PERMATA", "Bank Permata"},
	{"DANAMON", "Bank Danamon"},
	{"BTN", "Bank Tabungan Negara (BTN)"},
	{"PANIN", "Bank Panin"},
	{"OCBC", "OCBC NISP"},
	{"MAYBANK", "Maybank Indonesia"},
	{"BTPN", "Bank BTPN / Jenius"},
	{"MEGA", "Bank Mega"},
	{"BUKOPIN", "KB Bukopin"},
	{"DBS", "Bank DBS Indonesia"},
	{"SINARMAS", "Bank Sinarmas"},
	{"BJB", "Bank BJB"},
	{"JATIM", "Bank Jatim"},
	{"JAGO", "Bank Jago"},
	{"SEABANK", "SeaBank"},
	{"NEOCOMMERCE", "Bank Neo Commerce"},
	{"MUAMALAT", "Bank Muamalat"},
	{"BCA_SYARIAH", "BCA Syariah"},
}

// Banks mengembalikan salinan daftar bank yang didukung.
func Banks() []Bank { return append([]Bank(nil), banks...) }

func FindBank(code string) (Bank, bool) {
	for _, b := range banks {
		if b.Code == code {
			return b, true
		}
	}
	return Bank{}, false
}
