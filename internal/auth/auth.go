package auth

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type Repository interface {
	LoadAll() ([]User, error)
	Upsert(user User) error
	Remove(userID int64) error
}

type Service struct {
	repo         Repository
	allowedUsers map[int64]User
}

func NewWithRepo(repo Repository, initial []int64) (*Service, error) {
	s := &Service{repo: repo, allowedUsers: make(map[int64]User)}
	// preload from repo
	if repo != nil {
		users, err := repo.LoadAll()
		if err == nil {
			for _, u := range users {
				s.allowedUsers[u.ID] = u
			}
		}
	}
	// merge initial IDs (from env) without usernames
	for _, id := range initial {
		if _, ok := s.allowedUsers[id]; !ok {
			s.allowedUsers[id] = User{ID: id}
		}
	}
	return s, nil
}

func (s *Service) IsAllowed(userID int64) bool {
	_, ok := s.allowedUsers[userID]
	return ok
}

func (s *Service) Upsert(user User) error {
	s.allowedUsers[user.ID] = user
	if s.repo != nil {
		return s.repo.Upsert(user)
	}
	return nil
}

func (s *Service) Remove(userID int64) error {
	delete(s.allowedUsers, userID)
	if s.repo != nil {
		return s.repo.Remove(userID)
	}
	return nil
}

func (s *Service) List() []User {
	out := make([]User, 0, len(s.allowedUsers))
	for _, u := range s.allowedUsers {
		out = append(out, u)
	}
	return out
}
