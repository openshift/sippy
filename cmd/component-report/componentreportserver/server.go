package componentreportserver

import (
	"net/http"

	"github.com/openshift/sippy/pkg/db"
)

type Server struct {
	databaseConnection *db.DB
	httpServer         *http.ServeMux
}

func NewServer(databaseConnection *db.DB) *Server {
	s := &Server{
		databaseConnection: databaseConnection,
		httpServer:         http.NewServeMux(),
	}

	s.httpServer.HandleFunc("/componentsByJob", s.handleComponentsByJob)

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.httpServer.ServeHTTP(w, req)
}
