import path from 'node:path'
import { fileURLToPath } from 'node:url'

const srcRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../src')
const dataEvents = new Set(['DATA_CHANGED_EVENT', 'emitDataChanged', 'onDataChanged'])
const slash = (value) => value.split(path.sep).join('/')
const clean = (value) => value.split(/[?#]/, 1)[0].replace(/\.(?:vue|[cm]?[jt]sx?)$/, '')
const demo = (value) => /^(?:guide|demo|(?:components|composables|views)\/demo)(?:\/|$)/.test(value) ||
  /^(?:stores\/guide|components\/DemoConsole|api\/demo(?:Execution)?|api\/types\/demo|utils\/(?:demo[^/]*|stagedDemoState))(?:\/|$)/.test(value)
const assembly = (value) => /^(?:router|layouts|views\/login)(?:\/|$)/.test(value) || ['main', 'App'].includes(value)

function literal(node) {
  if (typeof node?.value === 'string') return node.value
  if (node?.type === 'TSLiteralType') return literal(node.literal)
  if (node?.type === 'TemplateLiteral' && !node.expressions.length) return node.quasis[0].value.cooked
  return null
}

function resolveImport(value, filename) {
  const name = value.split(/[?#]/, 1)[0]
  let absolute
  if (name.startsWith('@/')) absolute = path.resolve(srcRoot, './' + name.slice(2))
  else if (name.startsWith('/src/')) absolute = path.resolve(srcRoot, './' + name.slice(5))
  else if (name.startsWith('.')) absolute = path.resolve(path.dirname(filename), name)
  else return null
  return clean(slash(path.relative(srcRoot, absolute)))
}

export default {
  meta: {
    type: 'problem',
    schema: [],
    messages: {
      dependency: '业务与 Demo/Guide 通过 businessEvents 交互，不能直接导入该实现：{{name}}。',
      dynamic: '业务模块的动态导入必须使用固定模块路径，以便检查依赖方向。',
    },
  },
  create(context) {
    const filename = context.getPhysicalFilename()
    const source = clean(slash(path.relative(srcRoot, filename)))
    if (source.startsWith('../') || /\.(?:spec|test)$/.test(source)) return {}

    function check(sourceNode, node, specifiers = []) {
      if (!sourceNode) return
      const value = literal(sourceNode)
      if (value === null) {
        if (!assembly(source) && !demo(source)) context.report({ node, messageId: 'dynamic' })
        return
      }
      const target = resolveImport(value, filename)
      if (!target) return
      const eventStore = source === 'events/businessEvents' && target.startsWith('stores/')
      const demoPage = demo(source) && /^views\/(?!demo(?:\/|$)|login(?:\/|$))/.test(target)
      if (!eventStore && !demoPage && (assembly(source) || demo(source))) return
      // The public API type barrel is a shared contract, not a Demo runtime dependency.
      if (source === 'api/types/index' && target === 'api/types/demo') return
      const eventNames = specifiers.map((specifier) => specifier.type === 'ImportSpecifier'
        ? specifier.imported.name ?? specifier.imported.value
        : specifier.type === 'ExportSpecifier' ? specifier.local.name ?? specifier.local.value : null)
      const demoEvent = target === 'utils/events' && (!eventNames.length || !eventNames.every((name) => dataEvents.has(name)))
      if (eventStore || demoPage || demo(target) || demoEvent) context.report({ node, messageId: 'dependency', data: { name: value } })
    }
    return {
      ImportDeclaration: (node) => check(node.source, node, node.specifiers),
      ExportNamedDeclaration: (node) => check(node.source, node, node.specifiers),
      ExportAllDeclaration: (node) => check(node.source, node),
      ImportExpression: (node) => check(node.source, node),
      TSImportType: (node) => check(node.source ?? node.argument, node),
      TSImportEqualsDeclaration: (node) => check(node.moduleReference?.expression, node),
      CallExpression: (node) => {
        if (node.callee.type === 'Identifier' && node.callee.name === 'require') check(node.arguments[0], node)
      },
    }
  },
}
