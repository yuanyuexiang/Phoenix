import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert,
  Button,
  Card,
  Drawer,
  Flex,
  Form,
  Input,
  Layout,
  message,
  Select,
  Space,
  Table,
  Tag,
  Typography,
} from 'antd'
import type { ColumnsType } from 'antd/es/table'
import * as api from './api'
import type { Doc, DocType, Field } from './api'

const STATUS: Record<string, { text: string; color: string }> = {
  uploaded: { text: '已上传', color: 'default' },
  extracted: { text: '已提取', color: 'blue' },
  validated: { text: '校验通过', color: 'green' },
  needs_review: { text: '待人工审核', color: 'orange' },
  saved: { text: '已入库', color: 'success' },
  failed: { text: '失败', color: 'red' },
}

const StatusTag = ({ status }: { status: string }) => {
  const s = STATUS[status] ?? { text: status, color: 'default' }
  return <Tag color={s.color}>{s.text}</Tag>
}

export default function App() {
  const [msg, msgHolder] = message.useMessage()
  const [doctypes, setDoctypes] = useState<DocType[]>([])
  const [docs, setDocs] = useState<Doc[]>([])
  const [loading, setLoading] = useState(false)
  const [filters, setFilters] = useState({ doc_type: '', status: '', keyword: '' })
  const [current, setCurrent] = useState<Doc | null>(null)
  const [editedValues, setEditedValues] = useState<Record<string, string>>({})
  const [uploadForm] = Form.useForm()

  const fail = (e: unknown) => msg.error(e instanceof Error ? e.message : String(e))

  const loadDocs = useCallback(async () => {
    setLoading(true)
    try {
      const params: Record<string, string> = { limit: '50' }
      for (const [k, v] of Object.entries(filters)) if (v) params[k] = v
      const res = await api.queryDocuments(params)
      setDocs(res.documents ?? [])
    } catch (e) {
      fail(e)
    } finally {
      setLoading(false)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters])

  useEffect(() => {
    api.listDocTypes().then(setDoctypes).catch(fail)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    loadDocs()
  }, [loadDocs])

  const labelOf = useMemo(() => {
    return (doc: Doc, fieldName: string) => {
      const dt = doctypes.find((t) => t.name === doc.doc_type)
      return dt?.fields.find((f) => f.name === fieldName)?.label ?? fieldName
    }
  }, [doctypes])

  const openReview = (doc: Doc) => {
    setCurrent(doc)
    setEditedValues({})
  }

  const applyResult = (doc: Doc, tip: string) => {
    setCurrent(doc)
    setEditedValues({})
    msg.success(tip)
    loadDocs()
  }

  const reviewedFields = (): Field[] =>
    (current?.fields ?? []).map((f) => {
      const edited = editedValues[f.name]
      return edited === undefined || edited === f.value
        ? f
        : { name: f.name, value: edited, confidence: 1.0 } // 人工确认过的值置信度记为 1.0
    })

  const act = async (fn: () => Promise<Doc>, tip: string) => {
    try {
      applyResult(await fn(), tip)
    } catch (e) {
      fail(e)
    }
  }

  const onUpload = async (values: { doc_type: string; filename: string; content: string }) => {
    try {
      const doc = await api.uploadDocument(values.doc_type, values.filename, values.content)
      msg.success(`上传成功:${doc.id.slice(0, 8)}…`)
      uploadForm.resetFields(['content'])
      loadDocs()
    } catch (e) {
      fail(e)
    }
  }

  const columns: ColumnsType<Doc> = [
    { title: '文件名', dataIndex: 'filename', ellipsis: true },
    {
      title: '类型',
      dataIndex: 'doc_type',
      width: 140,
      render: (v: string) => doctypes.find((t) => t.name === v)?.title ?? v,
    },
    { title: '状态', dataIndex: 'status', width: 120, render: (v: string) => <StatusTag status={v} /> },
    { title: '创建时间', dataIndex: 'created_at', width: 170 },
    {
      title: '操作',
      width: 110,
      render: (_, doc) => <Button size="small" onClick={() => openReview(doc)}>查看/审核</Button>,
    },
  ]

  return (
    <Layout style={{ minHeight: '100vh' }}>
      {msgHolder}
      <Layout.Header style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
        <Typography.Title level={4} style={{ color: '#fff', margin: 0 }}>
          Phoenix 管理后台
        </Typography.Title>
        <Typography.Text style={{ color: 'rgba(255,255,255,.65)' }}>
          企业智能文档处理平台 · 人工审核 / 查询
        </Typography.Text>
      </Layout.Header>

      <Layout.Content style={{ maxWidth: 1100, width: '100%', margin: '24px auto', padding: '0 16px' }}>
        <Card title="上传测试文档" size="small" style={{ marginBottom: 16 }}>
          <Form form={uploadForm} onFinish={onUpload} initialValues={{ filename: 'test.txt' }}>
            <Space wrap>
              <Form.Item name="doc_type" rules={[{ required: true, message: '选择单据类型' }]} noStyle>
                <Select
                  placeholder="单据类型"
                  style={{ width: 200 }}
                  options={doctypes.map((t) => ({ value: t.name, label: `${t.title} (${t.name})` }))}
                />
              </Form.Item>
              <Form.Item name="filename" rules={[{ required: true }]} noStyle>
                <Input placeholder="文件名" style={{ width: 180 }} />
              </Form.Item>
              <Button type="primary" htmlType="submit">上传</Button>
            </Space>
            <Form.Item
              name="content"
              rules={[{ required: true, message: '请粘贴文本内容' }]}
              style={{ marginTop: 12, marginBottom: 0 }}
              extra="粘贴文本内容(演示用;WorkBuddy 侧走 MCP 的 file_url 上传)"
            >
              <Input.TextArea rows={4} placeholder={'编号: XXX-001\n标题: ……'} />
            </Form.Item>
          </Form>
        </Card>

        <Card title="文档列表" size="small">
          <Flex gap={8} wrap style={{ marginBottom: 12 }}>
            <Select
              placeholder="全部类型"
              allowClear
              style={{ width: 180 }}
              options={doctypes.map((t) => ({ value: t.name, label: t.title }))}
              onChange={(v) => setFilters((f) => ({ ...f, doc_type: v ?? '' }))}
            />
            <Select
              placeholder="全部状态"
              allowClear
              style={{ width: 160 }}
              options={Object.entries(STATUS).map(([value, s]) => ({ value, label: s.text }))}
              onChange={(v) => setFilters((f) => ({ ...f, status: v ?? '' }))}
            />
            <Input.Search
              placeholder="关键词(文件名/正文)"
              style={{ width: 240 }}
              allowClear
              onSearch={(v) => setFilters((f) => ({ ...f, keyword: v }))}
            />
            <Button onClick={loadDocs}>刷新</Button>
          </Flex>
          <Table rowKey="id" columns={columns} dataSource={docs} loading={loading} size="middle"
                 pagination={{ pageSize: 10, showSizeChanger: false }} />
        </Card>
      </Layout.Content>

      <Drawer
        title={
          current && (
            <Space>
              审核:{current.filename}
              <StatusTag status={current.status} />
            </Space>
          )
        }
        width={640}
        open={!!current}
        onClose={() => setCurrent(null)}
        footer={
          current && (
            <Space wrap>
              <Button onClick={() => act(() => api.extractDocument(current.id), '提取完成')}>重新提取</Button>
              <Button onClick={() => act(() => api.validateDocument(current.id), '校验完成')}>重新校验</Button>
              <Button
                type="primary"
                onClick={() => act(() => api.saveDocument(current.id, { fields: reviewedFields() }), '已入库')}
              >
                审核通过并入库
              </Button>
              <Button danger onClick={() => act(() => api.saveDocument(current.id, { force: true }), '已强制入库')}>
                强制入库
              </Button>
            </Space>
          )
        }
      >
        {current && (
          <>
            {current.error && <Alert type="error" message={current.error} style={{ marginBottom: 12 }} />}
            {(current.issues ?? []).length > 0 && (
              <Alert
                type="warning"
                message="校验问题"
                description={
                  <ul style={{ margin: 0, paddingLeft: 18 }}>
                    {current.issues!.map((i, idx) => (
                      <li key={idx}>{i.message}</li>
                    ))}
                  </ul>
                }
                style={{ marginBottom: 12 }}
              />
            )}
            <Table
              rowKey="name"
              size="small"
              pagination={false}
              dataSource={current.fields ?? []}
              locale={{ emptyText: '尚未提取字段' }}
              columns={[
                {
                  title: '字段',
                  dataIndex: 'name',
                  width: 170,
                  render: (name: string) => (
                    <>
                      {labelOf(current, name)}
                      <Typography.Text type="secondary" style={{ display: 'block', fontSize: 12 }}>
                        {name}
                      </Typography.Text>
                    </>
                  ),
                },
                {
                  title: '值(可修改)',
                  dataIndex: 'value',
                  render: (value: string, f: Field) => (
                    <Input
                      value={editedValues[f.name] ?? value}
                      onChange={(e) => setEditedValues((m) => ({ ...m, [f.name]: e.target.value }))}
                    />
                  ),
                },
                {
                  title: '置信度',
                  dataIndex: 'confidence',
                  width: 90,
                  render: (c: number) => (c ?? 0).toFixed(2),
                },
              ]}
            />
          </>
        )}
      </Drawer>
    </Layout>
  )
}
