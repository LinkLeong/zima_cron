package service

import (
  "github.com/IceWhaleTech/CasaOS-Common/external"
)

var (
  Gateway external.ManagementService
)

func Initialize(runtimePath string) {
  ms, err := external.NewManagementService(runtimePath)
  if err != nil && len(runtimePath) > 0 {
    panic(err)
  }
  Gateway = ms
}

