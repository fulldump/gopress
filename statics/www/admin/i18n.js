(function () {
  'use strict';
  const catalogs = {
    es: {
      'meta.title': 'GoPress · Administración', 'locale.label': 'Idioma de la interfaz',
      'nav.allBlogs': 'Todos los blogs', 'nav.posts': 'Entradas', 'nav.settings': 'Ajustes', 'nav.signOut': 'Salir',
      'blogs.eyebrow': 'Espacios', 'blogs.title': 'Tus blogs', 'blogs.intro': 'Elige un blog para escribir con tu equipo o crea uno nuevo.',
      'blogs.name': 'Nombre', 'blogs.address': 'Dirección', 'blogs.language': 'Idioma original', 'blogs.create': 'Crear blog',
      'blogs.creating': 'Creando', 'blogs.loading': 'Cargando blogs...', 'blogs.empty': 'Tu directorio está vacío',
      'blogs.emptyHelp': 'Crea el primer blog para empezar a publicar.',
      'posts.title': 'Entradas', 'posts.intro': 'Escribe, revisa y publica el trabajo editorial de este blog.',
      'posts.new': 'Nueva entrada', 'posts.published': 'Publicadas', 'posts.drafts': 'Borradores',
      'posts.publish': 'Publicar', 'posts.unpublish': 'Despublicar', 'posts.delete': 'Borrar',
      'settings.title': 'Ajustes', 'settings.intro': 'Configura la identidad y el equipo de este blog.', 'settings.identity': 'Identidad del blog', 'settings.identityHelp': 'Así se presenta el blog a sus lectores.', 'settings.targets': 'Traducir artículos a',
      'settings.targetsHelp': 'Las traducciones se generan con IA al publicar y nunca sustituyen al original.',
      'settings.save': 'Guardar cambios', 'settings.saving': 'Guardando...', 'settings.saved': 'Cambios guardados.',
      'settings.general': 'General', 'settings.members': 'Miembros', 'settings.invitations': 'Invitaciones',
      'settings.permanentAddress': 'Dirección permanente', 'settings.addressLocked': 'La dirección no puede cambiar después de crear el blog.',
      'settings.name': 'Nombre', 'settings.description': 'Descripción', 'settings.accent': 'Color de acento', 'settings.header': 'Imagen de cabecera',
      'settings.removeImage': 'Quitar imagen', 'settings.selectImage': 'Seleccionar imagen', 'settings.changeImage': 'Cambiar imagen',
      'members.help': 'Los editores pueden crear, editar y publicar entradas.', 'members.loading': 'Cargando miembros...', 'members.remove': 'Quitar', 'members.empty': 'No hay miembros.',
      'invitations.help': 'Cada enlace permite que una sola persona se una como editora.', 'invitations.create': 'Crear invitación',
      'invitations.loading': 'Cargando invitaciones...', 'invitations.pending': 'Invitación pendiente', 'invitations.revoke': 'Revocar', 'invitations.empty': 'No hay invitaciones pendientes.',
      'editor.title': 'Título del artículo', 'editor.publicUrl': 'URL pública', 'editor.tags': 'Tags',
      'editor.readOnly': 'Solo lectura', 'editor.view': 'Ver artículo ↗', 'editor.back': 'Entradas',
      'editor.sourceLanguage': 'Idioma original', 'editor.translations': 'Traducciones', 'editor.translationReady': 'Actualizada', 'editor.translationMissing': 'Pendiente',
      'editor.updateTranslations': 'Actualizar traducciones',
      'editor.translationsUnavailable': 'DeepSeek no está configurado; no se podrán generar traducciones al publicar.',
      'common.loading': 'Cargando...', 'common.owner': 'Propietario', 'common.editor': 'Editor',
      'pagination.loadMore': 'Cargar más', 'pagination.loading': 'Cargando...', 'pagination.failed': 'No se pudo cargar. Inténtalo de nuevo.',
      'language.es': 'Español', 'language.en': 'English', 'language.fr': 'Français', 'language.de': 'Deutsch',
      'language.pt': 'Português', 'language.zh': '中文'
    },
    en: {
      'meta.title': 'GoPress · Administration', 'locale.label': 'Interface language',
      'nav.allBlogs': 'All blogs', 'nav.posts': 'Posts', 'nav.settings': 'Settings', 'nav.signOut': 'Sign out',
      'blogs.eyebrow': 'Spaces', 'blogs.title': 'Your blogs', 'blogs.intro': 'Choose a blog to write with your team or create a new one.',
      'blogs.name': 'Name', 'blogs.address': 'Address', 'blogs.language': 'Original language', 'blogs.create': 'Create blog',
      'blogs.creating': 'Creating', 'blogs.loading': 'Loading blogs...', 'blogs.empty': 'Your directory is empty',
      'blogs.emptyHelp': 'Create your first blog to start publishing.',
      'posts.title': 'Posts', 'posts.intro': 'Write, review and publish this blog’s editorial work.',
      'posts.new': 'New post', 'posts.published': 'Published', 'posts.drafts': 'Drafts',
      'posts.publish': 'Publish', 'posts.unpublish': 'Unpublish', 'posts.delete': 'Delete',
      'settings.title': 'Settings', 'settings.intro': 'Configure this blog’s identity and team.', 'settings.identity': 'Blog identity', 'settings.identityHelp': 'How the blog appears to readers.', 'settings.targets': 'Translate articles into',
      'settings.targetsHelp': 'Translations are generated with AI on publish and never replace the original.',
      'settings.save': 'Save changes', 'settings.saving': 'Saving...', 'settings.saved': 'Changes saved.',
      'settings.general': 'General', 'settings.members': 'Members', 'settings.invitations': 'Invitations',
      'settings.permanentAddress': 'Permanent address', 'settings.addressLocked': 'The address cannot be changed after creating the blog.',
      'settings.name': 'Name', 'settings.description': 'Description', 'settings.accent': 'Accent color', 'settings.header': 'Header image',
      'settings.removeImage': 'Remove image', 'settings.selectImage': 'Select image', 'settings.changeImage': 'Change image',
      'members.help': 'Editors can create, edit and publish posts.', 'members.loading': 'Loading members...', 'members.remove': 'Remove', 'members.empty': 'No members.',
      'invitations.help': 'Each link allows one person to join as an editor.', 'invitations.create': 'Create invitation',
      'invitations.loading': 'Loading invitations...', 'invitations.pending': 'Pending invitation', 'invitations.revoke': 'Revoke', 'invitations.empty': 'No pending invitations.',
      'editor.title': 'Article title', 'editor.publicUrl': 'Public URL', 'editor.tags': 'Tags',
      'editor.readOnly': 'Read only', 'editor.view': 'View article ↗', 'editor.back': 'Posts',
      'editor.sourceLanguage': 'Original language', 'editor.translations': 'Translations', 'editor.translationReady': 'Up to date', 'editor.translationMissing': 'Pending',
      'editor.updateTranslations': 'Update translations',
      'editor.translationsUnavailable': 'DeepSeek is not configured; translations cannot be generated on publish.',
      'common.loading': 'Loading...', 'common.owner': 'Owner', 'common.editor': 'Editor',
      'pagination.loadMore': 'Load more', 'pagination.loading': 'Loading...', 'pagination.failed': 'Could not load more. Try again.',
      'language.es': 'Español', 'language.en': 'English', 'language.fr': 'Français', 'language.de': 'Deutsch',
      'language.pt': 'Português', 'language.zh': '中文'
    },
    zh: {
      'meta.title': 'GoPress · 管理后台', 'locale.label': '界面语言',
      'nav.allBlogs': '所有博客', 'nav.posts': '文章', 'nav.settings': '设置', 'nav.signOut': '退出',
      'blogs.eyebrow': '空间', 'blogs.title': '你的博客', 'blogs.intro': '选择一个博客与团队共同写作，或创建新博客。',
      'blogs.name': '名称', 'blogs.address': '地址', 'blogs.language': '原始语言', 'blogs.create': '创建博客',
      'blogs.creating': '正在创建', 'blogs.loading': '正在加载博客...', 'blogs.empty': '你的博客目录为空',
      'blogs.emptyHelp': '创建第一个博客，开始发布内容。',
      'posts.title': '文章', 'posts.intro': '撰写、审阅和发布此博客的内容。',
      'posts.new': '新建文章', 'posts.published': '已发布', 'posts.drafts': '草稿',
      'posts.publish': '发布', 'posts.unpublish': '取消发布', 'posts.delete': '删除',
      'settings.title': '设置', 'settings.intro': '配置此博客的形象和团队。', 'settings.identity': '博客信息', 'settings.identityHelp': '博客向读者展示的方式。', 'settings.targets': '将文章翻译为',
      'settings.targetsHelp': '发布时由 AI 生成翻译，且不会替换原文。',
      'settings.save': '保存更改', 'settings.saving': '正在保存...', 'settings.saved': '更改已保存。',
      'settings.general': '常规', 'settings.members': '成员', 'settings.invitations': '邀请',
      'settings.permanentAddress': '永久地址', 'settings.addressLocked': '博客创建后无法更改地址。',
      'settings.name': '名称', 'settings.description': '简介', 'settings.accent': '强调色', 'settings.header': '页眉图片',
      'settings.removeImage': '移除图片', 'settings.selectImage': '选择图片', 'settings.changeImage': '更换图片',
      'members.help': '编辑者可以创建、编辑和发布文章。', 'members.loading': '正在加载成员...', 'members.remove': '移除', 'members.empty': '暂无成员。',
      'invitations.help': '每个链接仅允许一人以编辑者身份加入。', 'invitations.create': '创建邀请',
      'invitations.loading': '正在加载邀请...', 'invitations.pending': '待接受邀请', 'invitations.revoke': '撤销', 'invitations.empty': '暂无待接受邀请。',
      'editor.title': '文章标题', 'editor.publicUrl': '公开 URL', 'editor.tags': '标签',
      'editor.readOnly': '只读', 'editor.view': '查看文章 ↗', 'editor.back': '文章',
      'editor.sourceLanguage': '原始语言', 'editor.translations': '翻译', 'editor.translationReady': '已更新', 'editor.translationMissing': '待更新',
      'editor.updateTranslations': '更新翻译',
      'editor.translationsUnavailable': 'DeepSeek 未配置，发布时无法生成翻译。',
      'common.loading': '正在加载...', 'common.owner': '所有者', 'common.editor': '编辑者',
      'pagination.loadMore': '加载更多', 'pagination.loading': '正在加载...', 'pagination.failed': '加载失败，请重试。',
      'language.es': '西班牙语', 'language.en': '英语', 'language.fr': '法语', 'language.de': '德语',
      'language.pt': '葡萄牙语', 'language.zh': '中文'
    }
  };
  function normalize(value) {
    const locale = String(value || '').toLowerCase().split(/[-_]/)[0];
    return ['es', 'en', 'zh'].includes(locale) ? locale : 'es';
  }
  let saved = '';
  try { saved = localStorage.getItem('gopress.admin.locale') || ''; } catch (_) {}
  const state = Vue.reactive({ locale: normalize(saved || (navigator.languages && navigator.languages[0]) || navigator.language) });
  function t(key) { return (catalogs[state.locale] && catalogs[state.locale][key]) || catalogs.es[key] || key; }
  function setLocale(locale) {
    state.locale = normalize(locale);
    document.documentElement.lang = state.locale;
    document.title = t('meta.title');
    try { localStorage.setItem('gopress.admin.locale', state.locale); } catch (_) {}
  }
  function editorMessages() {
    const message = (english, spanish, chinese) => state.locale === 'zh' ? chinese : state.locale === 'en' ? english : spanish;
    return { ui: { blockTunes: { toggler: { 'Click to tune': message('Click to tune', 'Opciones', '调整'), 'or drag to move': message('or drag to move', 'o arrastra para mover', '或拖动移动') } }, inlineToolbar: { converter: { 'Convert to': message('Convert to', 'Convertir a', '转换为') } }, toolbar: { toolbox: { Add: message('Add', 'Añadir', '添加'), Filter: message('Filter', 'Filtrar', '筛选'), 'Nothing found': message('Nothing found', 'Sin resultados', '未找到') } } }, toolNames: { Text: message('Text', 'Texto', '文本'), Heading: message('Heading', 'Encabezado', '标题'), List: message('List', 'Lista', '列表'), Checklist: message('Checklist', 'Lista de tareas', '任务清单'), Quote: message('Quote', 'Cita', '引用'), Warning: message('Warning', 'Aviso', '警告'), Code: message('Code', 'Código', '代码'), Delimiter: message('Delimiter', 'Separador', '分隔线'), 'Raw HTML': message('Raw HTML', 'HTML', '原始 HTML'), Table: message('Table', 'Tabla', '表格'), Link: message('Link', 'Enlace', '链接'), Image: message('Image', 'Imagen', '图片'), Attachment: message('Attachment', 'Adjunto', '附件') }, blockTunes: { delete: { Delete: message('Delete', 'Eliminar', '删除'), 'Click to delete': message('Click to delete', 'Confirma para eliminar', '确认删除') }, moveUp: { 'Move up': message('Move up', 'Subir', '上移') }, moveDown: { 'Move down': message('Move down', 'Bajar', '下移') } } };
  }
  setLocale(state.locale);
  window.AdminI18n = { state, t, setLocale, editorMessages, catalogs };
})();
