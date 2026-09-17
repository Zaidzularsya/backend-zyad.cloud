package service

import (
	"context"
	"math/big"
	"time"

	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/repository"
)

type ReportService struct {
	store LedgerStore
}

func NewReportService(store LedgerStore) *ReportService {
	return &ReportService{store: store}
}

var reportSectionLabels = map[string]string{
	"current_asset":         "Aset Lancar",
	"fixed_asset":           "Aset Tetap",
	"other_asset":           "Aset Lainnya",
	"current_liability":     "Kewajiban Jangka Pendek",
	"long_term_liability":   "Kewajiban Jangka Panjang",
	"equity":                "Ekuitas",
	"revenue":               "Pendapatan Usaha",
	"other_income":          "Pendapatan Lain-lain",
	"cogs":                  "Beban Pokok",
	"operating_expense":     "Beban Operasional",
	"other_expense":         "Beban Lain-lain",
	"current_year_earnings": "Laba (Rugi) Tahun Berjalan",
}

var profitLossIncreasingSections = map[string]bool{
	"revenue":      true,
	"other_income": true,
}

func (s *ReportService) ProfitLoss(ctx context.Context, query dto.ProfitLossQuery) (dto.ProfitLossResponse, error) {
	start, err := parseRequiredDate(query.StartDate)
	if err != nil {
		return dto.ProfitLossResponse{}, err
	}
	end, err := parseRequiredDate(query.EndDate)
	if err != nil {
		return dto.ProfitLossResponse{}, err
	}
	rows, err := s.store.ProfitLoss(ctx, start, end)
	if err != nil {
		return dto.ProfitLossResponse{}, err
	}
	sections := buildReportSections(rows)
	netIncome := netIncomeFromRows(rows)
	return dto.ProfitLossResponse{
		StartDate: formatDate(start), EndDate: formatDate(end),
		Sections: sections, NetIncome: formatMoney(netIncome),
	}, nil
}

func (s *ReportService) BalanceSheet(ctx context.Context, query dto.BalanceSheetQuery) (dto.BalanceSheetResponse, error) {
	asOf, err := parseRequiredDate(query.AsOfDate)
	if err != nil {
		return dto.BalanceSheetResponse{}, err
	}
	rows, err := s.store.BalanceSheet(ctx, asOf)
	if err != nil {
		return dto.BalanceSheetResponse{}, err
	}

	yearStart := time.Date(asOf.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
	plRows, err := s.store.ProfitLoss(ctx, yearStart, asOf)
	if err != nil {
		return dto.BalanceSheetResponse{}, err
	}
	netIncome := netIncomeFromRows(plRows)

	assetSections := make([]dto.ReportSectionResponse, 0)
	liabilitySections := make([]dto.ReportSectionResponse, 0)
	equitySections := buildReportSections(filterRowsBySection(rows, "equity"))
	totalAssets, totalLiabilities, totalEquity := zeroRat(), zeroRat(), zeroRat()

	for _, section := range buildReportSections(rows) {
		switch section.ReportSection {
		case "current_asset", "fixed_asset", "other_asset":
			assetSections = append(assetSections, section)
			amount, _ := moneyRat(section.Subtotal)
			totalAssets.Add(totalAssets, amount)
		case "current_liability", "long_term_liability":
			liabilitySections = append(liabilitySections, section)
			amount, _ := moneyRat(section.Subtotal)
			totalLiabilities.Add(totalLiabilities, amount)
		case "equity":
			amount, _ := moneyRat(section.Subtotal)
			totalEquity.Add(totalEquity, amount)
		}
	}

	equitySections = append(equitySections, dto.ReportSectionResponse{
		ReportSection: "current_year_earnings",
		Label:         reportSectionLabels["current_year_earnings"],
		Accounts: []dto.ReportLineResponse{{
			AccountName: reportSectionLabels["current_year_earnings"],
			Amount:      formatMoney(netIncome),
		}},
		Subtotal: formatMoney(netIncome),
	})
	totalEquity.Add(totalEquity, netIncome)

	return dto.BalanceSheetResponse{
		AsOfDate:             formatDate(asOf),
		AssetSections:        assetSections,
		LiabilitySections:    liabilitySections,
		EquitySections:       equitySections,
		CurrentYearNetIncome: formatMoney(netIncome),
		TotalAssets:          formatMoney(totalAssets),
		TotalLiabilities:     formatMoney(totalLiabilities),
		TotalEquity:          formatMoney(totalEquity),
		IsBalanced:           totalAssets.Cmp(new(big.Rat).Add(totalLiabilities, totalEquity)) == 0,
	}, nil
}

func filterRowsBySection(rows []repository.ReportAccountRow, section string) []repository.ReportAccountRow {
	filtered := make([]repository.ReportAccountRow, 0, len(rows))
	for _, row := range rows {
		if row.ReportSection == section {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

func buildReportSections(rows []repository.ReportAccountRow) []dto.ReportSectionResponse {
	sections := make([]dto.ReportSectionResponse, 0)
	index := map[string]int{}
	for _, row := range rows {
		amount, _ := moneyRat(row.Amount)
		i, ok := index[row.ReportSection]
		if !ok {
			label := reportSectionLabels[row.ReportSection]
			if label == "" {
				label = row.CategoryName
			}
			sections = append(sections, dto.ReportSectionResponse{
				ReportSection: row.ReportSection,
				Label:         label,
				Accounts:      []dto.ReportLineResponse{},
				Subtotal:      "0.00",
			})
			i = len(sections) - 1
			index[row.ReportSection] = i
		}
		sections[i].Accounts = append(sections[i].Accounts, dto.ReportLineResponse{
			AccountID: row.AccountID, AccountCode: row.AccountCode, AccountName: row.AccountName, Amount: row.Amount,
		})
		subtotal, _ := moneyRat(sections[i].Subtotal)
		subtotal.Add(subtotal, amount)
		sections[i].Subtotal = formatMoney(subtotal)
	}
	return sections
}

func netIncomeFromRows(rows []repository.ReportAccountRow) *big.Rat {
	net := zeroRat()
	for _, row := range rows {
		amount, err := moneyRat(row.Amount)
		if err != nil {
			continue
		}
		if profitLossIncreasingSections[row.ReportSection] {
			net.Add(net, amount)
		} else {
			net.Sub(net, amount)
		}
	}
	return net
}
