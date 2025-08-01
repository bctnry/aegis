//go:build ignore

package templates

import "github.com/bctnry/aegis/pkg/aegis"
import "github.com/bctnry/aegis/pkg/aegis/model"

type SettingGPGKeyTemplateModel struct {
	Config *aegis.AegisConfig
	LoginInfo *LoginInfoModel
	KeyList []model.AegisSigningKey
	ErrorMsg struct{
		Type string
		Message string
	}
		
}

