package service

import (
	"math/big"

	"zyad.cloud/internal/modules/finance/dto"
	"zyad.cloud/internal/modules/finance/model"
)

func zeroRat() *big.Rat {
	return new(big.Rat)
}

func accountResponse(a model.Account) dto.AccountResponse {
	return dto.AccountResponse{
		ID:                  a.ID,
		AccountCode:         a.AccountCode,
		AccountName:         a.AccountName,
		AccountCategoryID:   a.AccountCategoryID,
		AccountCategoryCode: a.AccountCategoryCode,
		AccountCategoryName: a.AccountCategoryName,
		AccountTypeCode:     a.AccountTypeCode,
		ParentAccountID:     a.ParentAccountID,
		IsHeader:            a.IsHeader,
		NormalBalance:       a.NormalBalance,
		IsActive:            a.IsActive,
		OpeningBalance:      a.OpeningBalance,
		OpeningBalanceDate:  formatOptionalDate(a.OpeningBalanceDate),
		Description:         a.Description,
		CreatedAt:           formatTime(a.CreatedAt),
		UpdatedAt:           formatTime(a.UpdatedAt),
	}
}

func fiscalYearResponse(y model.FiscalYear, periods []model.FiscalPeriod) dto.FiscalYearResponse {
	response := dto.FiscalYearResponse{
		ID:        y.ID,
		Year:      y.Year,
		StartDate: formatDate(y.StartDate),
		EndDate:   formatDate(y.EndDate),
		Status:    string(y.Status),
		ClosedAt:  formatOptionalTime(y.ClosedAt),
	}
	if periods != nil {
		response.Periods = make([]dto.FiscalPeriodResponse, 0, len(periods))
		for _, p := range periods {
			response.Periods = append(response.Periods, fiscalPeriodResponse(p))
		}
	}
	return response
}

func fiscalPeriodResponse(p model.FiscalPeriod) dto.FiscalPeriodResponse {
	return dto.FiscalPeriodResponse{
		ID:           p.ID,
		FiscalYearID: p.FiscalYearID,
		PeriodNumber: p.PeriodNumber,
		StartDate:    formatDate(p.StartDate),
		EndDate:      formatDate(p.EndDate),
		Status:       string(p.Status),
		ClosedAt:     formatOptionalTime(p.ClosedAt),
	}
}

func journalLineResponse(l model.JournalLine) dto.JournalLineResponse {
	return dto.JournalLineResponse{
		ID:            l.ID,
		LineNumber:    l.LineNumber,
		AccountID:     l.AccountID,
		AccountCode:   l.AccountCode,
		AccountName:   l.AccountName,
		Debit:         l.Debit,
		Credit:        l.Credit,
		Description:   l.Description,
		SubledgerType: string(l.SubledgerType),
		SubledgerID:   l.SubledgerID,
	}
}

func journalEntryResponse(e model.JournalEntry) dto.JournalEntryResponse {
	lines := make([]dto.JournalLineResponse, 0, len(e.Lines))
	totalDebitRat, totalCreditRat := zeroRat(), zeroRat()
	for _, l := range e.Lines {
		lines = append(lines, journalLineResponse(l))
		d, _ := moneyRat(l.Debit)
		c, _ := moneyRat(l.Credit)
		totalDebitRat.Add(totalDebitRat, d)
		totalCreditRat.Add(totalCreditRat, c)
	}
	return dto.JournalEntryResponse{
		ID:                e.ID,
		EntryNumber:       e.EntryNumber,
		EntryDate:         formatDate(e.EntryDate),
		FiscalPeriodID:    e.FiscalPeriodID,
		SourceType:        string(e.SourceType),
		SourceID:          e.SourceID,
		Reference:         e.Reference,
		Description:       e.Description,
		Status:            string(e.Status),
		PostedAt:          formatOptionalTime(e.PostedAt),
		ReversedByEntryID: e.ReversedByEntryID,
		Lines:             lines,
		TotalDebit:        formatMoney(totalDebitRat),
		TotalCredit:       formatMoney(totalCreditRat),
		CreatedAt:         formatTime(e.CreatedAt),
		UpdatedAt:         formatTime(e.UpdatedAt),
	}
}

func journalEntryResponses(entries []model.JournalEntry) []dto.JournalEntryResponse {
	responses := make([]dto.JournalEntryResponse, 0, len(entries))
	for _, e := range entries {
		responses = append(responses, journalEntryResponse(e))
	}
	return responses
}
