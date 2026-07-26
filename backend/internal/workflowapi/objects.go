package workflowapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/yuanyuexiang/phoenix/internal/api"
	"github.com/yuanyuexiang/phoenix/internal/ontology"
	"github.com/yuanyuexiang/phoenix/internal/store"
)

// 本体层端点(管理面;设计方案 §6):
//
//	GET  /api/ontology/types           对象类型清单(前端 tabs)
//	GET  /api/objects                  对象列表(type/keyword/property_filters/limit)
//	GET  /api/objects/{id}             对象详情(属性+出入链+证据文档)
//	GET  /api/documents/{id}/objects   某文档物化出的对象(文档/审核页联动 chips)
//	POST /api/ontology/rebuild         全量重建对象层(本体 YAML 大改后使用)

func (s *server) ontologyEnabled(w http.ResponseWriter) bool {
	if s.opts.OntReg == nil {
		writeError(w, http.StatusBadRequest, "本体层未启用(configs/ontology 为空)")
		return false
	}
	return true
}

func (s *server) ontologyTypes(w http.ResponseWriter, _ *http.Request) {
	types := []ontology.TypeInfo{}
	if s.opts.OntReg != nil {
		types = s.opts.OntReg.Types()
	}
	writeJSON(w, map[string]any{"types": types})
}

func (s *server) objectsList(w http.ResponseWriter, r *http.Request) {
	if !s.ontologyEnabled(w) {
		return
	}
	objects, err := ListObjectsForAPI(r, s.opts.DB)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]any{"total": len(objects), "objects": objects})
}

func (s *server) objectGet(w http.ResponseWriter, r *http.Request) {
	if !s.ontologyEnabled(w) {
		return
	}
	detail, err := ObjectDetailForAPI(r, s.opts.DB, s.opts.OntReg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if detail == nil {
		writeError(w, http.StatusNotFound, "对象不存在")
		return
	}
	writeJSON(w, detail)
}

func (s *server) documentObjects(w http.ResponseWriter, r *http.Request) {
	if !s.ontologyEnabled(w) {
		return
	}
	objs, err := s.opts.DB.ListObjectsByDocument(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if objs == nil {
		objs = []store.OntObject{}
	}
	writeJSON(w, map[string]any{"objects": objs})
}

func (s *server) ontologyRebuild(w http.ResponseWriter, r *http.Request) {
	n, warnings, err := s.opts.Pipeline.RebuildOntology(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.audit(r, "ontology_rebuild", "", map[string]any{"documents": n, "warnings": len(warnings)})
	if warnings == nil {
		warnings = []string{}
	}
	writeJSON(w, map[string]any{"documents": n, "warnings": warnings})
}

/* ---------- 两个 API 面共用的查询装配(restapi 亦调用) ---------- */

// ListObjectsForAPI 解析查询参数并执行对象列表查询(管理面与员工面共用)。
func ListObjectsForAPI(r *http.Request, db *store.DB) ([]store.OntObject, error) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	var propFilters []store.FieldFilter
	if raw := q.Get("property_filters"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &propFilters); err != nil {
			return nil, err
		}
	}
	objects, err := db.ListObjects(r.Context(), store.ObjectFilter{
		Type:        q.Get("type"),
		Keyword:     q.Get("keyword"),
		PropFilters: propFilters,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	if objects == nil {
		objects = []store.OntObject{}
	}
	return objects, nil
}

// ObjectDetailForAPI 组装对象详情:属性 + 出入链 + 证据文档(精简视图,不含字段)。
func ObjectDetailForAPI(r *http.Request, db *store.DB, reg *ontology.Registry) (map[string]any, error) {
	obj, out, in, docIDs, err := db.GetObject(r.Context(), r.PathValue("id"))
	if err != nil || obj == nil {
		return nil, err
	}
	typeTitle := obj.Type
	if ot, ok := reg.Get(obj.Type); ok {
		typeTitle = ot.Title
	}
	docs := []api.DocumentView{}
	for _, id := range docIDs {
		d, err := db.GetDocument(r.Context(), id)
		if err != nil {
			continue // 证据指向的文档已删(残留可由重建清理),跳过
		}
		v := api.ToView(d)
		v.Fields, v.Issues = nil, nil // 精简:联动列表不需要全量字段
		docs = append(docs, v)
	}
	if out == nil {
		out = []store.OntLink{}
	}
	if in == nil {
		in = []store.OntLink{}
	}
	return map[string]any{
		"object": obj, "type_title": typeTitle,
		"links_out": out, "links_in": in, "documents": docs,
	}, nil
}
