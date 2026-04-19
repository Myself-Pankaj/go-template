package visitorservice

// type VisitorService interface {
//     CreateEntry(ctx context.Context, req CreateVisitorRequest) (*VisitorEntry, error)
//     ApproveEntry(ctx context.Context, entryID int64, residentID int64) error
//     RejectEntry(ctx context.Context, entryID int64, residentID int64, reason string) error
//     MarkExit(ctx context.Context, entryID int64, guardID int64) error
//     GetSocietyEntries(ctx context.Context, societyID int64) ([]VisitorEntry, error)
//     GetFlatEntries(ctx context.Context, flatID int64) ([]VisitorEntry, error)
// }