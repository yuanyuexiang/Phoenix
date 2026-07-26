package restapi

import (
	"net/http"

	"github.com/yuanyuexiang/phoenix/internal/store"
	"github.com/yuanyuexiang/phoenix/internal/workflowapi"
)

// 本体层端点(员工面,查询只读;设计方案 §6/§7):
//
//	GET /pub/v1/objects                  对象列表
//	GET /pub/v1/objects/{id}             对象详情(属性+出入链+证据文档)
//	GET /pub/v1/documents/{id}/objects   某文档物化出的对象
//
// 对象的合并/修正/删除不对员工面开放(引导管理后台人工,与文档删除同策略)。

func (s *server) objectsList(w http.ResponseWriter, r *http.Request) {
	if s.opts.OntReg == nil {
		writeError(w, http.StatusBadRequest, "ONTOLOGY_DISABLED", "本体层未启用")
		return
	}
	objects, err := workflowapi.ListObjectsForAPI(r, s.opts.DB)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	writeJSON(w, map[string]any{"total": len(objects), "objects": objects})
}

func (s *server) objectGet(w http.ResponseWriter, r *http.Request) {
	if s.opts.OntReg == nil {
		writeError(w, http.StatusBadRequest, "ONTOLOGY_DISABLED", "本体层未启用")
		return
	}
	detail, err := workflowapi.ObjectDetailForAPI(r, s.opts.DB, s.opts.OntReg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "对象不存在")
		return
	}
	writeJSON(w, detail)
}

func (s *server) documentObjects(w http.ResponseWriter, r *http.Request) {
	if s.opts.OntReg == nil {
		writeError(w, http.StatusBadRequest, "ONTOLOGY_DISABLED", "本体层未启用")
		return
	}
	objs, err := s.opts.DB.ListObjectsByDocument(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if objs == nil {
		objs = []store.OntObject{}
	}
	writeJSON(w, map[string]any{"objects": objs})
}
