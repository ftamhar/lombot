package database

type PSETerdaftar struct {
	Url      string
	Location string
}

type PseResponse struct {
	Meta struct {
		Page struct {
			CurrentPage int `json:"currentPage"`
			From        int `json:"from"`
			LastPage    int `json:"lastPage"`
			PerPage     int `json:"perPage"`
			To          int `json:"to"`
			Total       int `json:"total"`
		} `json:"page"`
	} `json:"meta"`
	Data []struct {
		Type       string `json:"type"`
		ID         int    `json:"id"`
		Attributes struct {
			NomorPbUmku        interface{} `json:"nomor_pb_umku"`
			Nama               string      `json:"nama"`
			Website            string      `json:"website"`
			Sektor             string      `json:"sektor"`
			NamaPerusahaan     string      `json:"nama_perusahaan"`
			TanggalDaftar      string      `json:"tanggal_daftar"`
			NomorTandaDaftar   string      `json:"nomor_tanda_daftar"`
			QrCode             string      `json:"qr_code"`
			StatusID           string      `json:"status_id"`
			SistemElektronikID int         `json:"sistem_elektronik_id"`
		} `json:"attributes"`
	} `json:"data"`
}
