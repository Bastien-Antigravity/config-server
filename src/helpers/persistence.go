package helpers

import (
	"log"

	"github.com/Bastien-Antigravity/config-server/src/store"
)

// -----------------------------------------------------------------------------

func TryPersist(pm *store.PersistenceManager, s *store.Store) {
	if err := pm.Save(s.Get()); err != nil {
		log.Printf("Persistence failed: %v", err)
	}
}
