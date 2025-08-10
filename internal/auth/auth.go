package auth

type Service struct {
	allowedUsers map[int64]struct{}
}

func New(allowedUsers []int64) *Service {
	users := make(map[int64]struct{}, len(allowedUsers))
	for _, id := range allowedUsers {
		users[id] = struct{}{}
	}
	return &Service{allowedUsers: users}
}

func (s *Service) IsAllowed(userID int64) bool {
	_, ok := s.allowedUsers[userID]
	return ok
}
