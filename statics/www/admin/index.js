'use strict';

const nativeFetch = window.fetch.bind(window);
const localFrontend = ['localhost', '127.0.0.1', '::1'].includes(window.location.hostname);
let localAuthUserPromise = null;

function rememberLocalAuthUser(response) {
    if (!response.ok) return;
    localAuthUserPromise = response.clone().json()
        .then((payload) => payload && (payload.user || payload))
        .catch(() => null);
}

function loadLocalAuthUser() {
    if (!localAuthUserPromise) {
        localAuthUserPromise = nativeFetch('/auth/me', { headers: { Accept: 'application/json' } })
            .then((response) => response.ok ? response.json() : null)
            .then((payload) => payload && (payload.user || payload))
            .catch(() => null);
    }
    return localAuthUserPromise;
}

if (localFrontend) {
    window.fetch = async function localAuthenticatedFetch(input, options = {}) {
        const inputUrl = input instanceof Request ? input.url : String(input);
        const url = new URL(inputUrl, window.location.href);
        if (url.origin !== window.location.origin) return nativeFetch(input, options);

        if (url.pathname === '/auth/me') {
            const response = await nativeFetch(input, options);
            rememberLocalAuthUser(response);
            return response;
        }

        const user = await loadLocalAuthUser();
        const headers = new Headers(input instanceof Request ? input.headers : undefined);
        new Headers(options.headers || {}).forEach((value, name) => headers.set(name, value));
        if (user && user.id) {
            headers.set('X-Glue-Authentication', JSON.stringify({
                session: { id: 'local-development' },
                user: {
                    id: user.id,
                    nick: user.nick || '',
                    picture: user.picture || '',
                    email: user.email || '',
                },
            }));
        }
        return nativeFetch(input, { ...options, headers });
    };
}

const { createApp, reactive, nextTick } = Vue;

class HttpError extends Error {
    constructor(response, data) {
        const nodes = errorNodes(data);
        const message = firstErrorValue(nodes, ['description', 'message', 'title'])
            || `La petición falló (${response.status})`;
        super(String(message));
        this.name = 'HttpError';
        this.status = Number(firstErrorValue(nodes, ['status'])) || response.status;
        this.code = firstErrorValue(nodes, ['code']) || '';
        this.details = firstErrorValue(nodes, ['details']) || {};
        this.data = data;
    }
}

function errorNodes(value) {
    const nodes = [];
    const queue = [value];
    const seen = new Set();
    const nestedKeys = ['error', 'errors', 'data', 'cause'];
    while (queue.length && nodes.length < 20) {
        const node = queue.shift();
        if (!node || typeof node !== 'object' || seen.has(node)) continue;
        seen.add(node);
        nodes.push(node);
        nestedKeys.forEach((key) => {
            const nested = node[key];
            if (Array.isArray(nested)) queue.push(...nested);
            else if (nested && typeof nested === 'object') queue.push(nested);
        });
    }
    return nodes.reverse();
}

function firstErrorValue(nodes, keys) {
    for (const node of nodes) {
        for (const key of keys) {
            if (node[key] !== undefined && node[key] !== null && node[key] !== '') return node[key];
        }
    }
    return null;
}

let loginRedirectStarted = false;

function addLocalAuthHeader(headers) {
    if (['localhost', '127.0.0.1', '::1'].includes(window.location.hostname) && store.user && store.user.id) {
        headers.set('X-Glue-Authentication', JSON.stringify({
            session: { id: 'local-development' },
            user: {
                id: store.user.id,
                nick: store.user.nick || '',
                picture: store.user.picture || '',
                email: store.user.email || '',
            },
        }));
    }
    return headers;
}

async function request(path, options = {}) {
    const headers = new Headers(options.headers || {});
    addLocalAuthHeader(headers);
    headers.set('Accept', 'application/json');
    let body = options.body;
    const hasBody = body !== undefined && body !== null;
    if (hasBody && !(body instanceof FormData) && !(body instanceof Blob)) {
        headers.set('Content-Type', 'application/json');
        if (typeof body !== 'string') body = JSON.stringify(body);
    }

    const response = await fetch(path, {
        ...options,
        body,
        headers,
        credentials: 'same-origin',
    });
    const text = await response.text();
    let data = null;
    if (text) {
        try {
            data = JSON.parse(text);
        } catch (_) {
            data = { message: text };
        }
    }
    if (!response.ok) {
        if (response.status === 401 && path !== '/auth/me' && !loginRedirectStarted) {
            loginRedirectStarted = true;
            const target = currentLoginReturn();
            rememberLoginReturn(target);
            window.location.assign(`/auth/login?redirect=${encodeURIComponent(target)}`);
        }
        throw new HttpError(response, data);
    }
    return data;
}

function collection(data, key) {
    if (Array.isArray(data)) return data;
    if (data && Array.isArray(data[key])) return data[key];
    if (data && Array.isArray(data.items)) return data.items;
    if (data && data.data && data.data !== data) return collection(data.data, key);
    return [];
}

function entity(data, key) {
    if (!data) return data;
    if (data[key]) return data[key];
    if (data.data && !Array.isArray(data.data)) return entity(data.data, key);
    return data;
}

function idOf(value) {
    return value && (value.id || value.blogId || value.blog_id || value.articleId || value.article_id);
}

function roleOf(blog) {
    return blog && (blog.role || blog.memberRole || blog.membershipRole) || 'editor';
}

function articleIsPublished(article) {
    return Boolean(article && (article.published === true || article.status === 'published'));
}

function articleRevision(article) {
    return article && (article.revision !== undefined ? article.revision : article.version);
}

function articleSlug(value) {
    const normalized = String(value || '').toLowerCase().trim();
    let result = '';
    let lastHyphen = false;
    for (const character of normalized) {
        if (/[a-z0-9]/.test(character)) {
            result += character;
            lastHyphen = false;
        } else if (result && !lastHyphen) {
            result += '-';
            lastHyphen = true;
        }
    }
    result = result.replace(/^-+|-+$/g, '').slice(0, 80).replace(/-+$/g, '');
    return result || 'article';
}

function articleDate(article) {
    const raw = article && (article.updatedOn || article.updated_on || article.updatedAt
        || article.createdOn || article.created_on || article.createdAt);
    if (!raw) return 'Sin fecha';
    const date = new Date(raw);
    if (Number.isNaN(date.getTime())) return 'Sin fecha';
    return new Intl.DateTimeFormat('es', { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function errorMessage(error, fallback = 'Ha ocurrido un error.') {
    return error && error.message ? error.message : fallback;
}

function hasErrorCode(error, code) {
    return Boolean(error && error.code === code);
}

function lockDetails(error) {
    const details = error && error.details;
    return details && (details.lock || details.editLock || details.edit_lock || details);
}

function lockHolder(error) {
    const lock = lockDetails(error);
    return lock && (lock.holderNick || lock.holder_nick || lock.holderName || lock.holder_name) || '';
}

function lockMessage(error) {
    const holder = lockHolder(error);
    if (hasErrorCode(error, 'article_locked') || (error && error.status === 423)) {
        return holder ? `${holder} está editando este artículo.` : 'Otra persona está editando este artículo.';
    }
    return errorMessage(error, 'No se pudo reservar el artículo.');
}

function timestamp(value) {
    if (typeof value === 'number') return value < 100000000000 ? value * 1000 : value;
    const parsed = new Date(value).getTime();
    return Number.isNaN(parsed) ? 0 : parsed;
}

function absoluteUrl(value) {
    if (!value) return '';
    try {
        return new URL(value, window.location.origin).href;
    } catch (_) {
        return '';
    }
}

const clientId = crypto.randomUUID
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(16).slice(2)}`;

const store = reactive({
    user: null,
    blogs: [],
    blogsLoaded: false,
    blogsLoading: false,
    blogsPromise: null,
});

async function loadBlogs(force = false) {
    if (store.blogsPromise) {
        return force ? store.blogsPromise.then(() => loadBlogs(true)) : store.blogsPromise;
    }
    if (store.blogsLoaded && !force) return store.blogs;
    store.blogsLoading = true;
    store.blogsPromise = request('/v1/blogs')
        .then((data) => {
            store.blogs = collection(data, 'blogs');
            store.blogsLoaded = true;
            return store.blogs;
        })
        .finally(() => {
            store.blogsLoading = false;
            store.blogsPromise = null;
        });
    return store.blogsPromise;
}

const AppShell = {
    template: `
      <div class="app-shell">
        <header class="topbar">
          <router-link class="brand" :to="{ name: 'blogs' }">GoPress</router-link>
          <div class="blog-switcher">
            <label class="sr-only" for="blog-selector">Blog activo</label>
            <select id="blog-selector" :value="currentBlogId" @change="switchBlog">
              <option value="">Todos los blogs</option>
              <option v-for="blog in store.blogs" :key="idOf(blog)" :value="idOf(blog)">{{ blog.name }}</option>
            </select>
          </div>
          <div class="account" v-if="store.user">
            <span class="account-name">{{ store.user.nick || store.user.name || store.user.email }}</span>
            <img v-if="store.user.picture" class="avatar" :src="store.user.picture" alt="">
            <a class="logout" href="/auth/logout">Salir</a>
          </div>
        </header>
        <nav v-if="currentBlogId" class="blog-nav" aria-label="Administración del blog">
          <router-link :to="{ name: 'posts', params: { blogId: currentBlogId, filter: 'published' } }">Entradas</router-link>
          <router-link :to="{ name: 'settings', params: { blogId: currentBlogId } }">Ajustes</router-link>
        </nav>
        <div v-if="activeBlog && activeBlog.banned" class="ban-banner" role="alert">
          <strong>Este blog está baneado.</strong><span>No aparece públicamente y no puede crear ni publicar entradas.</span>
          <router-link :to="{ name: 'settings', params: { blogId: currentBlogId } }">Solicitar revisión</router-link>
        </div>
        <router-view v-slot="{ Component, route }">
          <component :is="Component" :key="viewKey(route)"></component>
        </router-view>
      </div>
    `,
    computed: {
        store() { return store; },
        currentBlogId() { return this.$route.params.blogId || ''; },
        activeBlog() { return store.blogs.find((blog) => String(idOf(blog)) === String(this.currentBlogId)) || null; },
    },
    mounted() {
        loadBlogs().catch(() => {});
    },
    methods: {
        idOf,
        viewKey(route) {
            const blogId = route.params.blogId || '';
            const articleId = route.params.articleId || '';
            const inviteToken = route.params.token || '';
            return `${String(route.name)}:${blogId}:${articleId}:${inviteToken}`;
        },
        switchBlog(event) {
            const blogId = event.target.value;
            if (!blogId) this.$router.push({ name: 'blogs' });
            else this.$router.push({ name: 'posts', params: { blogId, filter: 'published' } });
        },
    },
};

const BlogsPage = {
    template: `
      <main class="page">
        <div class="page-heading">
          <div><p class="eyebrow">Espacios</p><h1>Tus blogs</h1><p>Elige un blog para escribir con tu equipo o crea uno nuevo.</p></div>
        </div>
        <form class="panel create-blog" @submit.prevent="createBlog">
          <div class="field"><label for="blog-name">Nombre</label><input id="blog-name" v-model.trim="form.name" required maxlength="120" placeholder="Revista del equipo"></div>
          <div class="field"><label for="blog-slug">Dirección</label><div class="input-prefix"><span>/blog/</span><input id="blog-slug" v-model.trim="form.slug" required maxlength="63" pattern="[a-z0-9]+(?:-[a-z0-9]+)*" placeholder="revista-equipo"></div></div>
          <div class="field compact-field"><label for="blog-language">Idioma</label><select id="blog-language" v-model="form.language"><option value="es">Español</option><option value="en">English</option></select></div>
          <button class="button primary" :disabled="creating || !form.name || !form.slug"><span v-if="creating" class="spinner"></span>{{ creating ? 'Creando' : 'Crear blog' }}</button>
        </form>
        <p v-if="error" class="notice error" role="alert">{{ error }}</p>
        <div v-if="loading" class="state"><span class="spinner"></span> Cargando blogs...</div>
        <div v-else-if="!blogs.length" class="state empty-state"><strong>Tu directorio está vacío</strong><span>Crea el primer blog para empezar a publicar.</span></div>
        <div v-else class="blog-list">
          <router-link v-for="blog in blogs" :key="idOf(blog)" class="blog-row" :class="{ 'banned-blog-row': blog.banned }" :to="{ name: 'posts', params: { blogId: idOf(blog), filter: 'published' } }">
            <div><div class="row-title">{{ blog.name }}</div><div class="row-meta">/blog/{{ blog.slug }}</div></div>
            <span v-if="blog.banned" class="role-badge banned-badge">Baneado</span><span class="role-badge">{{ roleLabel(blog) }}</span><span class="row-arrow" aria-hidden="true">→</span>
          </router-link>
        </div>
      </main>
    `,
    data() {
        return { form: { name: '', slug: '', language: 'es' }, creating: false, error: '' };
    },
    computed: {
        blogs() { return store.blogs; },
        loading() { return store.blogsLoading && !store.blogsLoaded; },
    },
    async mounted() {
        try { await loadBlogs(); } catch (error) { this.error = errorMessage(error); }
    },
    methods: {
        idOf,
        roleLabel(blog) { return roleOf(blog) === 'owner' ? 'Propietario' : 'Editor'; },
        async createBlog() {
            this.creating = true;
            this.error = '';
            try {
                const result = await request('/v1/blogs', { method: 'POST', body: this.form });
                const blog = entity(result, 'blog');
                await loadBlogs(true);
                this.form = { name: '', slug: '', language: 'es' };
                if (idOf(blog)) this.$router.push({ name: 'posts', params: { blogId: idOf(blog), filter: 'published' } });
            } catch (error) {
                this.error = errorMessage(error);
            } finally {
                this.creating = false;
            }
        },
    },
};

const PostsPage = {
    template: `
      <main class="page">
        <div class="page-heading">
          <div><p class="eyebrow">{{ blogName }}</p><h1>Entradas</h1><p>Escribe, revisa y publica el trabajo editorial de este blog.</p></div>
          <button class="button primary" @click="createArticle" :disabled="creating || isBanned"><span v-if="creating" class="spinner"></span>{{ creating ? 'Creando' : 'Nueva entrada' }}</button>
        </div>
        <p v-if="isBanned" class="notice error" role="alert">Este blog está baneado. Sus entradas no son públicas y no se puede crear ni publicar contenido nuevo. Puedes solicitar una revisión desde Ajustes.</p>
        <div class="tabs" aria-label="Estado de las entradas">
          <router-link :to="{ name: 'posts', params: { blogId, filter: 'published' } }">Publicadas <span class="count">{{ published.length }}</span></router-link>
          <router-link :to="{ name: 'posts', params: { blogId, filter: 'drafts' } }">Borradores <span class="count">{{ drafts.length }}</span></router-link>
        </div>
        <p v-if="error" class="notice error" role="alert">{{ error }}</p>
        <div v-if="loading" class="state"><span class="spinner"></span> Cargando entradas...</div>
        <div v-else-if="!visibleArticles.length" class="state empty-state"><strong>{{ emptyTitle }}</strong><span>{{ emptyText }}</span></div>
        <div v-else class="article-list">
          <article v-for="article in visibleArticles" :key="idOf(article)" class="article-row">
            <router-link class="article-main" :to="{ name: 'edit', params: { blogId, articleId: idOf(article) } }">
              <div class="row-title">{{ article.title || 'Sin título' }}</div><div class="row-meta">{{ articleDate(article) }}</div>
            </router-link>
            <div class="row-actions">
              <button v-if="!articleIsPublished(article)" class="button small" @click="runAction(article, 'publish')" :disabled="isBusy(article) || isBanned">Publicar</button>
              <button v-else class="button small" @click="runAction(article, 'unpublish')" :disabled="isBusy(article)">Despublicar</button>
              <button class="button quiet small danger" @click="deleteArticle(article)" :disabled="isBusy(article)">Borrar</button>
            </div>
          </article>
        </div>
      </main>
    `,
    data() {
        return { articles: [], loading: true, creating: false, error: '', busy: {} };
    },
    computed: {
        blogId() { return this.$route.params.blogId; },
        blog() { return store.blogs.find((item) => String(idOf(item)) === String(this.blogId)) || null; },
        isBanned() { return Boolean(this.blog && this.blog.banned); },
        blogName() {
            return this.blog ? this.blog.name : 'Blog';
        },
        published() { return this.articles.filter(articleIsPublished); },
        drafts() { return this.articles.filter((article) => !articleIsPublished(article)); },
        visibleArticles() { return this.$route.params.filter === 'drafts' ? this.drafts : this.published; },
        emptyTitle() { return this.$route.params.filter === 'drafts' ? 'No hay borradores' : 'Todavía no has publicado'; },
        emptyText() { return this.$route.params.filter === 'drafts' ? 'Las nuevas entradas aparecerán aquí.' : 'Publica un borrador para verlo en esta sección.'; },
    },
    watch: {
        blogId: { immediate: true, handler() { this.loadArticles(); } },
    },
    methods: {
        idOf,
        articleDate,
        articleIsPublished,
        isBusy(article) { return Boolean(this.busy[idOf(article)]); },
        async loadArticles() {
            if (!this.blogId) return;
            this.loading = true;
            this.error = '';
            try {
                const data = await request(`/v1/blogs/${encodeURIComponent(this.blogId)}/articles`);
                this.articles = collection(data, 'articles').sort((a, b) => {
                    const aDate = a.updatedOn || a.updated_on || a.updatedAt || a.createdOn || a.created_on || '';
                    const bDate = b.updatedOn || b.updated_on || b.updatedAt || b.createdOn || b.created_on || '';
                    return String(bDate).localeCompare(String(aDate));
                });
            } catch (error) {
                this.error = errorMessage(error);
            } finally {
                this.loading = false;
            }
        },
        async createArticle() {
            this.creating = true;
            this.error = '';
            try {
                const result = await request(`/v1/blogs/${encodeURIComponent(this.blogId)}/articles`, { method: 'POST', body: { title: '' } });
                const article = entity(result, 'article');
                if (!idOf(article)) throw new Error('El servidor no devolvió el identificador del artículo.');
                this.$router.push({ name: 'edit', params: { blogId: this.blogId, articleId: idOf(article) } });
            } catch (error) {
                this.error = errorMessage(error);
            } finally {
                this.creating = false;
            }
        },
        async withShortLock(article, operation) {
            const articleId = idOf(article);
            const root = `/v1/blogs/${encodeURIComponent(this.blogId)}/articles/${encodeURIComponent(articleId)}`;
            let token = '';
            this.busy[articleId] = true;
            this.error = '';
            try {
                const lease = await request(`${root}/edit-lock/acquire`, { method: 'POST', body: { clientId } });
                token = lease && lease.token;
                if (!token) throw new Error('El servidor no devolvió el bloqueo de edición.');
                return { ok: true, value: await operation(root, token) };
            } catch (error) {
                if (hasErrorCode(error, 'article_locked') || error.status === 423) this.error = lockMessage(error);
                else if (hasErrorCode(error, 'revision_conflict')) this.error = 'La entrada cambió. Actualiza la lista antes de volver a intentarlo.';
                else if (hasErrorCode(error, 'edit_lock_lost')) this.error = 'El bloqueo caducó antes de completar la acción. Inténtalo de nuevo.';
                else this.error = errorMessage(error);
                return { ok: false, value: null };
            } finally {
                if (token) await releaseLease(root, token).catch(() => {});
                delete this.busy[articleId];
            }
        },
        async runAction(article, action) {
            const revision = articleRevision(article);
            if (revision === undefined || revision === null) {
                this.error = 'La entrada no incluye una revisión y no se puede modificar de forma segura.';
                return;
            }
            const result = await this.withShortLock(article, (root, token) => request(`${root}/${action}`, {
                method: 'POST', headers: { 'X-Edit-Lock-Token': token }, body: { revision },
            }));
            if (result.ok) await this.loadArticles();
        },
        async deleteArticle(article) {
            if (!window.confirm(`¿Borrar “${article.title || 'Sin título'}”? Esta acción no se puede deshacer.`)) return;
            const revision = articleRevision(article);
            if (revision === undefined || revision === null) {
                this.error = 'La entrada no incluye una revisión y no se puede borrar de forma segura.';
                return;
            }
            const result = await this.withShortLock(article, (root, token) => request(root, {
                method: 'DELETE', headers: { 'X-Edit-Lock-Token': token }, body: { revision },
            }));
            if (result.ok) this.articles = this.articles.filter((item) => idOf(item) !== idOf(article));
        },
    },
};

const SettingsPage = {
    template: `
      <main class="page">
        <div class="page-heading"><div><p class="eyebrow">Personalización y acceso</p><h1>Ajustes</h1><p>Configura la identidad y el equipo de este blog.</p></div></div>
        <div v-if="loading" class="state"><span class="spinner"></span> Cargando ajustes...</div>
        <p v-else-if="loadError" class="notice error" role="alert">{{ loadError }}</p>
        <template v-else>
          <p v-if="!isOwner" class="notice warning" role="status">Tienes acceso de editor. Puedes consultar estos ajustes, pero solo el propietario puede modificarlos.</p>
          <p v-if="error" class="notice error" role="alert">{{ error }}</p>
          <section v-if="blog.banned && isOwner" class="ban-review-panel" aria-labelledby="review-title">
            <p class="eyebrow">Moderación</p><h2 id="review-title">Blog baneado</h2>
            <p>El blog y todas sus entradas están ocultos de las páginas públicas. No puedes crear ni publicar contenido hasta que se apruebe una revisión.</p>
            <div v-if="blog.review_requested_on" class="notice" role="status"><strong>Revisión solicitada.</strong> El equipo de moderación revisará el blog.</div>
            <form v-else class="review-form" @submit.prevent="requestReview">
              <div class="field"><label for="review-message">Explica por qué debe revisarse el baneo</label><textarea id="review-message" v-model="reviewMessage" required minlength="10" maxlength="2000" placeholder="Describe los cambios realizados o el motivo de la solicitud."></textarea><small>Entre 10 y 2000 caracteres.</small></div>
              <button class="button primary" :disabled="reviewSubmitting || reviewMessage.trim().length < 10">{{ reviewSubmitting ? 'Enviando...' : 'Solicitar revisión' }}</button>
              <p v-if="reviewError" class="notice error" role="alert">{{ reviewError }}</p>
            </form>
          </section>
          <div class="settings-layout" :class="{ 'read-only-settings': !isOwner }">
            <nav class="settings-index" aria-label="Secciones de ajustes"><a href="#general" @click.prevent="scrollTo('general')">General</a><a v-if="isOwner" href="#members" @click.prevent="scrollTo('members')">Miembros</a><a v-if="isOwner" href="#invitations" @click.prevent="scrollTo('invitations')">Invitaciones</a></nav>
            <div class="settings-content">
              <section id="general" class="settings-section">
                <h2>Identidad del blog</h2><p>Así se presenta el blog a sus lectores.</p>
                <form class="settings-form" @submit.prevent="saveBlog">
                  <div class="field"><span class="field-label">Dirección permanente</span><div class="readonly-value">/blog/{{ blog.slug }}</div><small>La dirección no puede cambiar después de crear el blog.</small></div>
                  <div class="field"><label v-if="isOwner" for="setting-name">Nombre</label><span v-else class="field-label">Nombre</span><input v-if="isOwner" id="setting-name" v-model.trim="form.name" required maxlength="120"><div v-else class="readonly-value">{{ form.name || 'Sin nombre' }}</div></div>
                  <div class="field"><label v-if="isOwner" for="setting-description">Descripción</label><span v-else class="field-label">Descripción</span><textarea v-if="isOwner" id="setting-description" v-model="form.description" maxlength="2000"></textarea><div v-else class="readonly-value multiline">{{ form.description || 'Sin descripción' }}</div></div>
                  <div class="settings-grid">
                    <div class="field"><label v-if="isOwner" for="setting-language">Idioma</label><span v-else class="field-label">Idioma</span><select v-if="isOwner" id="setting-language" v-model="form.language"><option value="es">Español</option><option value="en">English</option></select><div v-else class="readonly-value">{{ form.language === 'en' ? 'English' : 'Español' }}</div></div>
                    <div class="field"><label v-if="isOwner" for="setting-accent">Color de acento</label><span v-else class="field-label">Color de acento</span><div v-if="isOwner" class="color-field"><input id="setting-accent" type="color" v-model="form.accent_color"><input v-model.trim="form.accent_color" pattern="#[0-9a-fA-F]{6}" aria-label="Color de acento hexadecimal"></div><div v-else class="readonly-value color-value"><span class="color-swatch" :style="{ backgroundColor: form.accent_color }"></span>{{ form.accent_color }}</div></div>
                  </div>
                  <div class="field">
                    <span class="field-label">Imagen de cabecera</span>
                    <div v-if="form.header_image" class="header-image-preview"><img :src="form.header_image" alt="Previsualización de la cabecera"><button v-if="isOwner" type="button" class="button small danger" @click="removeHeaderImage" :disabled="headerUploading">Quitar imagen</button></div>
                    <template v-if="isOwner">
                      <label class="button file-picker" for="setting-header">{{ headerUploading ? 'Subiendo imagen...' : (form.header_image ? 'Cambiar imagen' : 'Seleccionar imagen') }}<input id="setting-header" type="file" accept="image/jpeg,image/png,image/gif,image/webp,image/avif" @change="uploadHeaderImage" :disabled="headerUploading"></label>
                      <small>JPEG, PNG, GIF, WebP o AVIF. Máximo 25 MB. La imagen se almacena en este blog.</small>
                      <small v-if="headerUploadError" class="field-error" role="alert">{{ headerUploadError }}</small>
                    </template>
                    <div v-else-if="!form.header_image" class="readonly-value">Sin imagen</div>
                  </div>
                  <div v-if="isOwner" class="form-actions"><button class="button primary" :disabled="saving || headerUploading">{{ saving ? 'Guardando...' : 'Guardar cambios' }}</button><span v-if="saved" class="help" role="status">Cambios guardados.</span></div>
                </form>
              </section>

              <section v-if="isOwner" id="members" class="settings-section">
                <h2>Miembros</h2><p>Los editores pueden crear, editar y publicar entradas.</p>
                <div v-if="membersLoading" class="state"><span class="spinner"></span> Cargando miembros...</div>
                <div v-else class="member-list">
                  <div v-for="member in members" :key="memberKey(member)" class="member-row">
                    <div><div class="member-name">{{ memberName(member) }}</div><div v-if="member.email || member.user_email" class="row-meta">{{ member.email || member.user_email }}</div></div>
                    <span class="member-role role-badge">{{ member.role === 'owner' ? 'Propietario' : 'Editor' }}</span>
                    <button v-if="member.role !== 'owner'" class="button quiet small danger" @click="removeMember(member)" :disabled="memberBusy === memberUserId(member)">Quitar</button>
                  </div>
                  <div v-if="!members.length" class="state">No hay miembros.</div>
                </div>
              </section>

              <section v-if="isOwner" id="invitations" class="settings-section">
                <h2>Invitaciones</h2><p>Cada enlace permite que una sola persona se una como editora.</p>
                <div class="invite-create"><button class="button primary" @click="createInvitation" :disabled="inviteCreating">{{ inviteCreating ? 'Creando...' : 'Crear invitación' }}</button></div>
                <div v-if="newInviteLink" class="invite-link"><input :value="newInviteLink" readonly aria-label="Enlace de invitación"><button class="button small" @click="copyInvite(newInviteLink)">{{ copied ? 'Copiado' : 'Copiar' }}</button></div>
                <div v-if="invitesLoading" class="state"><span class="spinner"></span> Cargando invitaciones...</div>
                <div v-else class="member-list">
                  <div v-for="invite in invitations" :key="inviteId(invite)" class="member-row">
                    <div><div class="member-name">Invitación pendiente</div><div class="row-meta">{{ inviteDate(invite) }}</div></div><span class="member-role role-badge">Un uso</span>
                    <button class="button quiet small danger" @click="revokeInvitation(invite)" :disabled="inviteBusy === inviteId(invite)">Revocar</button>
                  </div>
                  <div v-if="!invitations.length" class="state">No hay invitaciones pendientes.</div>
                </div>
              </section>
            </div>
          </div>
        </template>
      </main>
    `,
    data() {
        return {
            blog: null, form: {}, members: [], invitations: [], loading: true, loadError: '', error: '',
            membersLoading: false, invitesLoading: false, saving: false, saved: false, memberBusy: '',
            inviteBusy: '', inviteCreating: false, newInviteLink: '', newInviteId: '', copied: false,
            headerUploading: false, headerUploadError: '',
            reviewMessage: '', reviewSubmitting: false, reviewError: '',
        };
    },
    computed: {
        blogId() { return this.$route.params.blogId; },
        isOwner() { return roleOf(this.blog) === 'owner'; },
    },
    async mounted() { await this.load(); },
    methods: {
        async load() {
            this.loading = true;
            this.loadError = '';
            try {
                const [result] = await Promise.all([
                    request(`/v1/blogs/${encodeURIComponent(this.blogId)}`),
                    loadBlogs().catch(() => []),
                ]);
                const listed = store.blogs.find((item) => String(idOf(item)) === String(this.blogId)) || {};
                this.blog = { ...listed, ...entity(result, 'blog') };
                this.form = {
                    name: this.blog.name || '',
                    description: this.blog.description || '',
                    language: this.blog.language || 'es',
                    accent_color: this.blog.accent_color || this.blog.accentColor || '#176b4d',
                    header_image: this.blog.header_image || this.blog.headerImage || '',
                };
                if (this.isOwner) await Promise.all([this.loadMembers(), this.loadInvitations()]);
            } catch (error) {
                this.loadError = errorMessage(error);
            } finally {
                this.loading = false;
            }
        },
        async saveBlog() {
            if (!this.isOwner) return;
            this.saving = true;
            this.saved = false;
            this.error = '';
            try {
                const result = await request(`/v1/blogs/${encodeURIComponent(this.blogId)}`, { method: 'PATCH', body: { ...this.form } });
                this.blog = { ...this.blog, ...entity(result, 'blog') };
                this.saved = true;
                await loadBlogs(true);
            } catch (error) {
                this.error = errorMessage(error);
            } finally {
                this.saving = false;
            }
        },
        async uploadHeaderImage(event) {
            const input = event.target;
            const file = input.files && input.files[0];
            input.value = '';
            if (!file) return;
            const allowed = ['image/jpeg', 'image/png', 'image/gif', 'image/webp', 'image/avif'];
            if (!allowed.includes(file.type)) {
                this.headerUploadError = 'Selecciona una imagen JPEG, PNG, GIF, WebP o AVIF.';
                return;
            }
            if (file.size > 25 * 1024 * 1024) {
                this.headerUploadError = 'La imagen no puede superar los 25 MB.';
                return;
            }
            this.headerUploading = true;
            this.headerUploadError = '';
            this.saved = false;
            try {
                const body = new FormData();
                body.append('file', file, file.name);
                const result = await request(`/v1/files?blogId=${encodeURIComponent(this.blogId)}`, { method: 'POST', body });
                const uploaded = entity(result, 'file');
                if (!uploaded || !uploaded.url) throw new Error('La subida no devolvió una URL válida.');
                this.form.header_image = uploaded.url;
            } catch (error) {
                this.headerUploadError = errorMessage(error, 'No se pudo subir la imagen.');
            } finally {
                this.headerUploading = false;
            }
        },
        removeHeaderImage() {
            this.form.header_image = '';
            this.headerUploadError = '';
            this.saved = false;
        },
        async requestReview() {
            if (!this.isOwner || !this.blog.banned) return;
            this.reviewSubmitting = true;
            this.reviewError = '';
            try {
                const result = await request(`/v1/blogs/${encodeURIComponent(this.blogId)}/review-request`, {
                    method: 'POST', body: { message: this.reviewMessage },
                });
                this.blog = { ...this.blog, ...entity(result, 'blog') };
                this.reviewMessage = '';
                await loadBlogs(true);
            } catch (error) {
                this.reviewError = errorMessage(error, 'No se pudo solicitar la revisión.');
            } finally {
                this.reviewSubmitting = false;
            }
        },
        async loadMembers() {
            this.membersLoading = true;
            try {
                const data = await request(`/v1/blogs/${encodeURIComponent(this.blogId)}/members`);
                this.members = collection(data, 'members');
            } catch (error) {
                this.error = errorMessage(error);
            } finally {
                this.membersLoading = false;
            }
        },
        async loadInvitations() {
            this.invitesLoading = true;
            try {
                const data = await request(`/v1/blogs/${encodeURIComponent(this.blogId)}/invitations`);
                this.invitations = collection(data, 'invitations').filter((invite) => !invite.status || invite.status === 'pending');
            } catch (error) {
                this.error = errorMessage(error);
            } finally {
                this.invitesLoading = false;
            }
        },
        scrollTo(id) { document.getElementById(id)?.scrollIntoView({ behavior: 'smooth', block: 'start' }); },
        memberUserId(member) { return member.user_id || member.userId || (member.user && idOf(member.user)); },
        memberKey(member) { return member.id || this.memberUserId(member); },
        memberName(member) { return member.user_nick || member.userNick || member.nick || member.name || (member.user && (member.user.nick || member.user.name)) || 'Miembro'; },
        inviteId(invite) { return invite.id || invite.inviteId || invite.invitationId; },
        inviteDate(invite) { return articleDate({ createdOn: invite.createdOn || invite.created_on || invite.createdAt || invite.expiresOn || invite.expiresAt }); },
        invitationLink(invite) {
            const supplied = invite && (invite.inviteUrl || invite.acceptUrl || invite.acceptanceUrl);
            if (supplied) return absoluteUrl(supplied);
            const token = invite && invite.token;
            return token ? absoluteUrl(`${window.location.pathname}#/invite/${encodeURIComponent(token)}`) : '';
        },
        async removeMember(member) {
            const userId = this.memberUserId(member);
            if (!userId || !window.confirm(`¿Quitar a ${this.memberName(member)} del blog?`)) return;
            this.memberBusy = userId;
            this.error = '';
            try {
                await request(`/v1/blogs/${encodeURIComponent(this.blogId)}/members/${encodeURIComponent(userId)}`, { method: 'DELETE' });
                this.members = this.members.filter((item) => this.memberUserId(item) !== userId);
            } catch (error) {
                this.error = errorMessage(error);
            } finally {
                this.memberBusy = '';
            }
        },
        async createInvitation() {
            this.inviteCreating = true;
            this.error = '';
            try {
                const result = await request(`/v1/blogs/${encodeURIComponent(this.blogId)}/invitations`, { method: 'POST' });
                const invite = entity(result, 'invitation');
                const inviteWithUrl = result && result.inviteUrl && invite !== result ? { ...invite, inviteUrl: result.inviteUrl } : invite;
                this.newInviteLink = this.invitationLink(inviteWithUrl);
                this.newInviteId = this.inviteId(invite);
                this.copied = false;
                await this.loadInvitations();
            } catch (error) {
                this.error = errorMessage(error);
            } finally {
                this.inviteCreating = false;
            }
        },
        async copyInvite(link) {
            try {
                await navigator.clipboard.writeText(absoluteUrl(link));
                this.copied = true;
            } catch (_) {
                const input = document.createElement('textarea');
                input.value = absoluteUrl(link);
                input.style.position = 'fixed';
                input.style.opacity = '0';
                document.body.appendChild(input);
                input.select();
                document.execCommand('copy');
                input.remove();
                this.copied = true;
            }
        },
        async revokeInvitation(invite) {
            const invitationId = this.inviteId(invite);
            this.inviteBusy = invitationId;
            this.error = '';
            try {
                await request(`/v1/blogs/${encodeURIComponent(this.blogId)}/invitations/${encodeURIComponent(invitationId)}`, { method: 'DELETE' });
                this.invitations = this.invitations.filter((item) => this.inviteId(item) !== invitationId);
                if (invitationId === this.newInviteId) {
                    this.newInviteId = '';
                    this.newInviteLink = '';
                }
            } catch (error) {
                this.error = errorMessage(error);
            } finally {
                this.inviteBusy = '';
            }
        },
    },
};

const InvitePage = {
    template: `
      <main class="page narrow invite-page">
        <section class="panel invite-box">
          <p class="eyebrow">Colaboración</p><h1>Invitación a un blog</h1>
          <p v-if="state === 'ready'">Acepta para unirte como editor. Este enlace solo puede utilizarse una vez.</p>
          <p v-else-if="state === 'success'" class="notice" role="status">Ya formas parte del blog.</p>
          <p v-else-if="state === 'error'" class="notice error" role="alert">{{ error }}</p>
          <button v-if="state === 'ready'" class="button primary" @click="accept">Aceptar invitación</button>
          <button v-else-if="state === 'loading'" class="button primary" disabled><span class="spinner"></span>Aceptando...</button>
          <router-link v-else-if="state === 'success' && blogId" class="button primary" :to="{ name: 'posts', params: { blogId, filter: 'published' } }">Ir al blog</router-link>
          <router-link v-else class="button" :to="{ name: 'blogs' }">Volver al directorio</router-link>
        </section>
      </main>
    `,
    data() { return { state: 'ready', error: '', blogId: '' }; },
    methods: {
        async accept() {
            this.state = 'loading';
            try {
                const result = await request('/v1/invitations/accept', { method: 'POST', body: { token: this.$route.params.token } });
                const accepted = entity(result, 'blog') || {};
                this.blogId = idOf(accepted) || accepted.blog_id || accepted.blogId;
                this.state = 'success';
                loadBlogs(true).catch(() => {});
            } catch (error) {
                this.error = errorMessage(error);
                this.state = 'error';
            }
        },
    },
};

async function releaseLease(root, token, keepalive = false) {
    if (!token) return;
    const headers = addLocalAuthHeader(new Headers({ 'X-Edit-Lock-Token': token, 'Accept': 'application/json' }));
    await fetch(`${root}/edit-lock/release`, {
        method: 'POST',
        headers,
        credentials: 'same-origin',
        keepalive,
    });
}

const EditorPage = {
    template: `
      <main class="editor-page">
        <div v-if="loading" class="page"><div class="state"><span class="spinner"></span> Cargando artículo...</div></div>
        <template v-else-if="article">
          <div class="editor-toolbar">
            <div class="editor-toolbar-left">
              <router-link class="button quiet small" :to="{ name: 'posts', params: { blogId, filter: articleIsPublished(article) ? 'published' : 'drafts' } }">← <span class="button-label">Entradas</span></router-link>
              <a v-if="articleIsPublished(article) && publicUrl" class="button quiet small public-link" :href="publicUrl" target="_blank" rel="noopener">Ver artículo ↗</a>
            </div>
            <div class="editor-toolbar-actions">
              <span class="save-state" :class="saveState" aria-live="polite">{{ saveLabel }}</span>
              <button v-if="saveState === 'error'" class="button small" @click="retrySave" :disabled="lockState !== 'editable'">Reintentar</button>
              <button v-if="!articleIsPublished(article)" class="button primary small" @click="publish" :disabled="actionBusy || lockState !== 'editable' || isBanned">{{ actionBusy ? 'Publicando...' : 'Publicar' }}</button>
              <button v-else class="button small" @click="unpublish" :disabled="actionBusy || lockState !== 'editable'">{{ actionBusy ? 'Procesando...' : 'Despublicar' }}</button>
            </div>
          </div>
          <div class="editor-wrap">
            <div v-if="lockNotice" class="notice" :class="lockNoticeClass" :role="lockState === 'conflict' || lockState === 'lost' ? 'alert' : 'status'">
              <span>{{ lockNotice }}</span>
              <button v-if="canRetryLock" class="button quiet small" @click="acquireLock">Intentar de nuevo</button>
              <button v-if="lockState === 'conflict'" class="button quiet small" @click="reloadFromServer">Descartar y recargar</button>
            </div>
            <p v-if="error" class="notice error" role="alert">{{ error }}</p>
            <div class="editor-heading"><input class="title-input" v-model="article.title" @input="handleTitleInput" :disabled="lockState !== 'editable'" maxlength="240" placeholder="Título del artículo" aria-label="Título del artículo"></div>
            <div class="editor-url field">
              <div class="editor-url-label"><label for="article-url">URL pública</label><span v-if="article.automatic_url && !articleIsPublished(article)" class="url-mode">Automática</span></div>
              <div class="input-prefix"><span>{{ publicUrlPrefix }}</span><input id="article-url" v-model.trim="article.url" @input="handleUrlInput" :disabled="lockState !== 'editable'" maxlength="80" pattern="[a-z0-9]+(?:-[a-z0-9]+)*" spellcheck="false" autocomplete="off" aria-describedby="article-url-help"></div>
              <div id="article-url-help" class="editor-url-help">
                <small v-if="articleIsPublished(article)">La entrada está publicada. Cambiar esta URL romperá los enlaces anteriores.</small>
                <small v-else-if="article.automatic_url">Se actualiza automáticamente a partir del título hasta que publiques.</small>
                <small v-else>URL personalizada. Puedes volver a generarla desde el título.</small>
                <button v-if="!articleIsPublished(article) && !article.automatic_url" type="button" class="url-mode-button" @click="useAutomaticUrl" :disabled="lockState !== 'editable'">Usar el título automáticamente</button>
                <button v-else-if="articleIsPublished(article)" type="button" class="url-mode-button" @click="useTitleUrl" :disabled="lockState !== 'editable'">Generar desde el título</button>
              </div>
            </div>
            <div class="editor-surface" :class="{ 'is-readonly': lockState !== 'editable' }">
              <div v-if="lockState !== 'editable'" class="readonly-overlay" aria-hidden="true">Solo lectura</div>
              <div id="editorjs"></div>
            </div>
          </div>
        </template>
        <div v-else class="page"><div class="state error" role="alert">{{ error || 'No se pudo cargar el artículo.' }}</div></div>
      </main>
    `,
    data() {
        return {
            article: null, editor: null, loading: true, error: '', actionBusy: false, actionPromise: null,
            lockState: 'loading', lockToken: '', lockError: null, lockWarning: '', leaseExpiresAt: 0,
            heartbeatAfterSeconds: 25, heartbeatTimer: null, leaseExpiryTimer: null, renewPromise: null, renewAgain: false,
            serverOffset: 0, acquiringPromise: null, saveTimer: null, savePromise: null,
            changeGeneration: 0, savedGeneration: 0, saveState: 'saved', destroyed: false,
            navigationPending: false,
        };
    },
    computed: {
        blogId() { return this.$route.params.blogId; },
        articleId() { return this.$route.params.articleId; },
        isBanned() {
            const blog = store.blogs.find((item) => String(idOf(item)) === String(this.blogId));
            return Boolean(blog && blog.banned);
        },
        articleRoot() { return `/v1/blogs/${encodeURIComponent(this.blogId)}/articles/${encodeURIComponent(this.articleId)}`; },
        saveLabel() {
            return {
                saved: 'Guardado', pending: 'Cambios pendientes', saving: 'Guardando...', error: 'Error al guardar',
                lost: 'Sin bloqueo', conflict: 'Conflicto',
            }[this.saveState] || 'Guardado';
        },
        lockNotice() {
            if (this.lockState === 'acquiring') return 'Reservando la edición de este artículo...';
            if (this.lockState === 'blocked') return lockMessage(this.lockError);
            if (this.lockState === 'lost') return 'Se perdió el bloqueo de edición. Tus cambios locales siguen en esta página y no se han sobrescrito.';
            if (this.lockState === 'conflict') return 'El artículo cambió en el servidor. Tus cambios locales siguen visibles; recargar requiere confirmación.';
            return this.lockWarning;
        },
        lockNoticeClass() {
            return this.lockState === 'acquiring' ? 'info' : 'warning';
        },
        canRetryLock() {
            return (this.lockState === 'blocked' || this.lockState === 'lost') && !this.acquiringPromise;
        },
        publicUrl() {
            if (!this.article || !this.article.url || this.isBanned) return '';
            return this.publicUrlPrefix ? `${this.publicUrlPrefix}${encodeURIComponent(this.article.url)}` : '';
        },
        blogSlug() {
            const blog = store.blogs.find((item) => String(idOf(item)) === String(this.blogId));
            return this.article && (this.article.blog_slug || this.article.blogSlug) || (blog && blog.slug) || '';
        },
        publicUrlPrefix() {
            return this.blogSlug ? `/blog/${encodeURIComponent(this.blogSlug)}/article/` : '';
        },
    },
    async mounted() {
        window.addEventListener('keydown', this.handleKeydown);
        window.addEventListener('beforeunload', this.handleBeforeUnload);
        window.addEventListener('pagehide', this.handlePageHide);
        document.addEventListener('visibilitychange', this.handleVisibilityChange);
        await this.loadArticle();
    },
    beforeUnmount() {
        this.destroyed = true;
        window.removeEventListener('keydown', this.handleKeydown);
        window.removeEventListener('beforeunload', this.handleBeforeUnload);
        window.removeEventListener('pagehide', this.handlePageHide);
        document.removeEventListener('visibilitychange', this.handleVisibilityChange);
        clearTimeout(this.saveTimer);
        clearTimeout(this.heartbeatTimer);
        clearTimeout(this.leaseExpiryTimer);
        if (this.lockToken && !this.isDirty() && !this.savePromise && !this.actionPromise) this.releaseLock(true);
        this.destroyEditor();
    },
    beforeRouteLeave(to, from, next) {
        this.prepareNavigation(next);
    },
    beforeRouteUpdate(to, from, next) {
        this.prepareNavigation(next);
    },
    methods: {
        articleIsPublished,
        async loadArticle() {
            const root = this.articleRoot;
            this.loading = true;
            this.error = '';
            this.lockState = 'loading';
            try {
                const result = await request(root);
                if (this.destroyed || root !== this.articleRoot) return;
                this.article = entity(result, 'article');
                this.changeGeneration = 0;
                this.savedGeneration = 0;
                this.saveState = 'saved';
                this.loading = false;
                this.lockState = 'acquiring';
                await nextTick();
                this.createEditor();
                await this.acquireLock();
            } catch (error) {
                if (this.destroyed) return;
                this.error = errorMessage(error);
                this.loading = false;
                this.lockState = 'lost';
            }
        },
        editorData() {
            let data = this.article && this.article.content && this.article.content.data;
            if (typeof data === 'string') {
                try { data = JSON.parse(data); } catch (_) { data = null; }
            }
            if (!data && this.article && this.article.content && Array.isArray(this.article.content.blocks)) data = this.article.content;
            if (!data || !Array.isArray(data.blocks)) return { blocks: [] };
            return {
                ...data,
                blocks: data.blocks.map((block) => {
                    if (!block || (block.type !== 'image' && block.type !== 'simpleImage')) return block;
                    const blockData = { ...(block.data || {}) };
                    if (!blockData.file && blockData.url) blockData.file = { url: blockData.url };
                    return { ...block, type: 'image', data: blockData };
                }),
            };
        },
        createEditor() {
            const fileEndpoint = `/v1/files?blogId=${encodeURIComponent(this.blogId)}`;
            this.editor = new EditorJS({
                holder: 'editorjs',
                placeholder: 'Empieza a escribir...',
                autofocus: false,
                readOnly: true,
                data: this.editorData(),
                tools: {
                    header: { class: Header, inlineToolbar: ['marker', 'link'], config: { placeholder: 'Encabezado' }, shortcut: 'CMD+SHIFT+H' },
                    list: { class: EditorjsList, inlineToolbar: true, shortcut: 'CMD+SHIFT+L' },
                    checklist: { class: Checklist, inlineToolbar: true },
                    quote: { class: Quote, inlineToolbar: true, config: { quotePlaceholder: 'Cita', captionPlaceholder: 'Autor' }, shortcut: 'CMD+SHIFT+O' },
                    warning: Warning,
                    marker: { class: Marker, shortcut: 'CMD+SHIFT+M' },
                    code: { class: CodeTool, shortcut: 'CMD+SHIFT+C' },
                    delimiter: Delimiter,
                    inlineCode: { class: InlineCode, shortcut: 'CMD+SHIFT+C' },
                    linkTool: { class: LinkTool, config: { endpoint: '/v1/editor/helperFetchUrl', headers: { Accept: 'application/json' } } },
                    embed: Embed,
                    table: { class: Table, inlineToolbar: true, shortcut: 'CMD+ALT+T' },
                    raw: RawTool,
                    underline: Underline,
                    image: {
                        class: ImageTool,
                        config: {
                            endpoints: { byFile: fileEndpoint },
                            additionalRequestHeaders: Object.fromEntries(addLocalAuthHeader(new Headers({ Accept: 'application/json' }))),
                        },
                    },
                    attaches: { class: AttachesTool, config: { endpoint: fileEndpoint, additionalRequestHeaders: Object.fromEntries(addLocalAuthHeader(new Headers({ Accept: 'application/json' }))) } },
                },
                onChange: () => this.markDirty(),
            });
            this.editor.isReady.catch((error) => {
                if (!this.destroyed) this.error = errorMessage(error, 'No se pudo iniciar el editor.');
            });
        },
        destroyEditor() {
            const editor = this.editor;
            this.editor = null;
            if (!editor) return;
            Promise.resolve(editor.isReady)
                .then(() => { if (editor.destroy) editor.destroy(); })
                .catch(() => {});
        },
        async setEditorReadOnly(readOnly) {
            const editor = this.editor;
            if (!editor) return;
            try {
                await editor.isReady;
                if (editor.readOnly && editor.readOnly.toggle) await editor.readOnly.toggle(readOnly);
            } catch (_) {}
        },
        async acquireLock() {
            if (this.acquiringPromise || this.lockToken || this.destroyed) return false;
            const root = this.articleRoot;
            this.lockState = 'acquiring';
            this.lockError = null;
            this.lockWarning = '';
            this.acquiringPromise = request(`${root}/edit-lock/acquire`, { method: 'POST', body: { clientId } });
            try {
                const lease = await this.acquiringPromise;
                const token = lease && lease.token;
                if (!token) throw new Error('El servidor no devolvió el bloqueo de edición.');
                if (this.destroyed || root !== this.articleRoot) {
                    await releaseLease(root, token, true).catch(() => {});
                    return false;
                }
                this.applyLease(lease);
                await this.setEditorReadOnly(false);
                if (this.destroyed) {
                    await releaseLease(root, token, true).catch(() => {});
                    return false;
                }
                this.lockState = 'editable';
                if (this.isDirty()) {
                    this.saveState = 'pending';
                    this.scheduleSave(1000);
                }
                return true;
            } catch (error) {
                if (this.destroyed) return false;
                this.lockToken = '';
                this.lockError = error;
                this.lockState = hasErrorCode(error, 'article_locked') || error.status === 423 ? 'blocked' : 'lost';
                await this.setEditorReadOnly(true);
                return false;
            } finally {
                this.acquiringPromise = null;
            }
        },
        applyLease(lease) {
            const serverTime = timestamp(lease.serverTime);
            if (serverTime) this.serverOffset = serverTime - Date.now();
            this.lockToken = lease.token || this.lockToken;
            this.leaseExpiresAt = timestamp(lease.expiresAt) || (Date.now() + this.serverOffset + (Number(lease.ttlSeconds) || 90) * 1000);
            this.heartbeatAfterSeconds = Number(lease.heartbeatAfterSeconds) || 25;
            clearTimeout(this.leaseExpiryTimer);
            const expiresIn = this.leaseExpiresAt - (Date.now() + this.serverOffset);
            this.leaseExpiryTimer = setTimeout(() => {
                if (this.lockToken) this.loseLock(new Error('El bloqueo de edición ha caducado.'));
            }, Math.max(0, expiresIn));
            this.scheduleHeartbeat(this.heartbeatAfterSeconds * 1000);
        },
        scheduleHeartbeat(delay) {
            clearTimeout(this.heartbeatTimer);
            if (!this.lockToken || this.destroyed) return;
            this.heartbeatTimer = setTimeout(() => this.renewLock(), Math.max(1000, delay));
        },
        async renewLock(force = false) {
            if (!this.lockToken || this.destroyed) return false;
            if (this.renewPromise) {
                if (force) this.renewAgain = true;
                return this.renewPromise;
            }
            clearTimeout(this.heartbeatTimer);
            const token = this.lockToken;
            const root = this.articleRoot;
            this.renewPromise = request(`${root}/edit-lock/renew`, { method: 'POST', headers: { 'X-Edit-Lock-Token': token } });
            try {
                const lease = await this.renewPromise;
                if (this.destroyed || token !== this.lockToken || root !== this.articleRoot) return false;
                this.lockWarning = '';
                this.applyLease(lease || {});
                return true;
            } catch (error) {
                if (this.destroyed || token !== this.lockToken) return false;
                const serverNow = Date.now() + this.serverOffset;
                if (error.status === 401 || hasErrorCode(error, 'edit_lock_lost') || serverNow >= this.leaseExpiresAt - 1000) {
                    await this.loseLock(error);
                } else {
                    this.lockWarning = 'No se pudo renovar el bloqueo. Se volverá a intentar en unos segundos.';
                    this.scheduleHeartbeat(5000);
                }
                return false;
            } finally {
                this.renewPromise = null;
                if (this.renewAgain && this.lockToken && !this.destroyed) {
                    this.renewAgain = false;
                    this.renewLock();
                }
            }
        },
        async loseLock(error) {
            clearTimeout(this.heartbeatTimer);
            clearTimeout(this.leaseExpiryTimer);
            clearTimeout(this.saveTimer);
            this.heartbeatTimer = null;
            this.leaseExpiryTimer = null;
            this.saveTimer = null;
            this.lockToken = '';
            this.lockError = error;
            this.lockState = 'lost';
            if (this.isDirty()) this.saveState = 'lost';
            await this.setEditorReadOnly(true);
        },
        async setConflict(error) {
            clearTimeout(this.saveTimer);
            this.saveTimer = null;
            this.lockError = error;
            this.lockState = 'conflict';
            this.saveState = 'conflict';
            await this.setEditorReadOnly(true);
        },
        isDirty() { return this.changeGeneration > this.savedGeneration; },
        handleTitleInput() {
            if (!articleIsPublished(this.article) && this.article.automatic_url) {
                this.article.url = articleSlug(this.article.title);
            }
            this.markDirty();
        },
        handleUrlInput() {
            this.article.automatic_url = false;
            this.markDirty();
        },
        useAutomaticUrl() {
            this.article.automatic_url = true;
            this.article.url = articleSlug(this.article.title);
            this.markDirty();
        },
        useTitleUrl() {
            this.article.automatic_url = false;
            this.article.url = articleSlug(this.article.title);
            this.markDirty();
        },
        markDirty() {
            if (this.lockState !== 'editable' || this.destroyed) return;
            this.changeGeneration += 1;
            this.saveState = 'pending';
            this.error = '';
            this.scheduleSave(1000);
        },
        scheduleSave(delay) {
            clearTimeout(this.saveTimer);
            this.saveTimer = setTimeout(() => this.drainSave(), delay);
        },
        async drainSave() {
            clearTimeout(this.saveTimer);
            this.saveTimer = null;
            if (this.savePromise) return this.savePromise;
            if (!this.isDirty()) return true;
            if (this.lockState !== 'editable' || !this.lockToken || !this.editor) return false;
            const revision = articleRevision(this.article);
            if (revision === undefined || revision === null) {
                this.error = 'El artículo no incluye una revisión y no se puede guardar de forma segura.';
                this.saveState = 'error';
                return false;
            }
            const generation = this.changeGeneration;
            const token = this.lockToken;
            const title = this.article.title;
            const url = this.article.url;
            const automaticUrl = Boolean(this.article.automatic_url);
            const root = this.articleRoot;
            let succeeded = false;
            this.saveState = 'saving';
            this.savePromise = (async () => {
                try {
                    const content = await this.editor.save();
                    const result = await request(root, {
                        method: 'PATCH',
                        headers: { 'X-Edit-Lock-Token': token },
                        body: { revision, title, url, automatic_url: automaticUrl, content: { type: 'editorjs', data: content } },
                    });
                    const updated = entity(result, 'article') || {};
                    if (updated.revision !== undefined) this.article.revision = updated.revision;
                    if (updated.url !== undefined && generation === this.changeGeneration) this.article.url = updated.url;
                    if (updated.automatic_url !== undefined && generation === this.changeGeneration) this.article.automatic_url = updated.automatic_url;
                    if (updated.published !== undefined) this.article.published = updated.published;
                    this.savedGeneration = Math.max(this.savedGeneration, generation);
                    succeeded = true;
                    this.error = '';
                    this.saveState = this.isDirty() ? 'pending' : 'saved';
                    return true;
                } catch (error) {
                    this.error = errorMessage(error);
                    if (hasErrorCode(error, 'revision_conflict')) await this.setConflict(error);
                    else if (hasErrorCode(error, 'edit_lock_lost') || hasErrorCode(error, 'article_locked')) await this.loseLock(error);
                    else this.saveState = 'error';
                    return false;
                } finally {
                    this.savePromise = null;
                    if (succeeded && this.isDirty() && this.lockState === 'editable') this.scheduleSave(0);
                }
            })();
            return this.savePromise;
        },
        async flushSave() {
            clearTimeout(this.saveTimer);
            this.saveTimer = null;
            if (this.savePromise) await this.savePromise;
            while (this.isDirty()) {
                if (this.lockState !== 'editable' || !this.lockToken) return false;
                const saved = await this.drainSave();
                if (!saved) return false;
                if (this.savePromise) await this.savePromise;
            }
            return !this.savePromise;
        },
        async retrySave() {
            if (this.lockState !== 'editable') return;
            this.error = '';
            this.saveState = 'pending';
            await this.flushSave();
        },
        async mutatePublication(action) {
            if (this.actionPromise) return this.actionPromise;
            this.actionBusy = true;
            this.error = '';
            this.actionPromise = (async () => {
                try {
                    if (this.lockState !== 'editable' || !await this.flushSave()) return false;
                    const revision = articleRevision(this.article);
                    if (revision === undefined || revision === null) {
                        this.error = 'El artículo no incluye una revisión y no se puede publicar de forma segura.';
                        return false;
                    }
                    const result = await request(`${this.articleRoot}/${action}`, {
                        method: 'POST',
                        headers: { 'X-Edit-Lock-Token': this.lockToken },
                        body: { revision },
                    });
                    const updated = entity(result, 'article') || {};
                    this.article = { ...this.article, ...updated, published: action === 'publish' };
                    return true;
                } catch (error) {
                    this.error = errorMessage(error);
                    if (hasErrorCode(error, 'revision_conflict')) await this.setConflict(error);
                    else if (hasErrorCode(error, 'edit_lock_lost') || hasErrorCode(error, 'article_locked')) await this.loseLock(error);
                    return false;
                } finally {
                    this.actionBusy = false;
                    this.actionPromise = null;
                }
            })();
            return this.actionPromise;
        },
        publish() { return this.mutatePublication('publish'); },
        unpublish() { return this.mutatePublication('unpublish'); },
        async reloadFromServer() {
            if (this.isDirty() && !window.confirm('¿Descartar tus cambios locales y cargar la versión del servidor?')) return;
            this.savedGeneration = this.changeGeneration;
            await this.releaseLock();
            this.destroyEditor();
            this.article = null;
            await this.loadArticle();
        },
        handleKeydown(event) {
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
                event.preventDefault();
                this.flushSave();
            }
        },
        handleVisibilityChange() {
            if (document.visibilityState === 'visible' && this.lockToken) this.renewLock(true);
        },
        handleBeforeUnload(event) {
            if (!this.isDirty() && !this.savePromise && !this.actionPromise) return;
            event.preventDefault();
            event.returnValue = '';
        },
        handlePageHide() {
            if (!this.isDirty() && !this.savePromise && !this.actionPromise) this.releaseLock(true);
        },
        async releaseLock(keepalive = false) {
            const token = this.lockToken;
            const root = this.articleRoot;
            if (!token) return;
            this.lockToken = '';
            clearTimeout(this.heartbeatTimer);
            clearTimeout(this.leaseExpiryTimer);
            this.heartbeatTimer = null;
            this.leaseExpiryTimer = null;
            await releaseLease(root, token, keepalive).catch(() => {});
        },
        async prepareNavigation(next) {
            if (this.navigationPending) {
                next(false);
                return;
            }
            this.navigationPending = true;
            let proceed = false;
            try {
                if (this.actionPromise) await this.actionPromise;
                const clean = await this.flushSave();
                if (clean) {
                    proceed = true;
                } else if (window.confirm('No se pudieron guardar todos los cambios. ¿Salir y descartarlos?')) {
                    this.savedGeneration = this.changeGeneration;
                    proceed = true;
                }
                if (proceed) await this.releaseLock();
            } finally {
                this.navigationPending = false;
                next(proceed);
            }
        },
    },
};

const routes = [
    { path: '/', name: 'blogs', component: BlogsPage },
    {
        path: '/blogs/:blogId/posts/:filter', name: 'posts', component: PostsPage,
        beforeEnter: (to) => ['published', 'drafts'].includes(to.params.filter)
            || { name: 'posts', params: { blogId: to.params.blogId, filter: 'published' } },
    },
    { path: '/blogs/:blogId/edit/:articleId', name: 'edit', component: EditorPage },
    { path: '/blogs/:blogId/settings', name: 'settings', component: SettingsPage },
    { path: '/invite/:token', name: 'invite', component: InvitePage },
    { path: '/:pathMatch(.*)*', redirect: { name: 'blogs' } },
];

const router = VueRouter.createRouter({
    history: VueRouter.createWebHashHistory(),
    routes,
    scrollBehavior(to) { return to.hash ? { el: to.hash, top: 140 } : { top: 0 }; },
});

function currentLoginReturn() {
    return window.location.pathname + window.location.search + window.location.hash;
}

function rememberLoginReturn(target) {
    try { window.sessionStorage.setItem('gopressLoginReturn', target); } catch (_) {}
}

function resumeLoginReturn() {
    let target = '';
    try {
        target = window.sessionStorage.getItem('gopressLoginReturn') || '';
        window.sessionStorage.removeItem('gopressLoginReturn');
    } catch (_) {
        return false;
    }
    if (!target) return false;
    try {
        const destination = new URL(target, window.location.origin);
        if (destination.origin !== window.location.origin || destination.href === window.location.href) return false;
        window.location.replace(destination.href);
        return true;
    } catch (_) {
        return false;
    }
}

let userPromise = null;
let loginReturnChecked = false;
router.beforeEach(async () => {
    if (!store.user) {
        try {
            if (!userPromise) {
                userPromise = request('/auth/me').then((result) => {
                    const user = entity(result, 'user');
                    if (!user || user.error || !user.id) throw new Error('No hay una sesión activa.');
                    store.user = user;
                    return user;
                }).finally(() => { userPromise = null; });
            }
            await userPromise;
        } catch (_) {
            const target = currentLoginReturn();
            rememberLoginReturn(target);
            window.location.assign(`/auth/login?redirect=${encodeURIComponent(target)}`);
            return false;
        }
    }
    if (!loginReturnChecked) {
        loginReturnChecked = true;
        if (resumeLoginReturn()) return false;
    }
    return true;
});

createApp(AppShell).use(router).mount('#app');
