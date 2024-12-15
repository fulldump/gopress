
// 1. Define route components.
// These can be imported from other files
const Home = {
    data() {
        return {};
    },
    template: `
    <div id="page-home">
 
      <div
        style="background: black; color: white; padding: 16px;"
      >
        Barra del usuario
      </div>
    
      Aquí están tus posts: 
      
        <router-link
          :to="{ name: 'listPosts', params: { filter: 'published' } }"
        >
            Publicados
            <span class="counter">29</span>
        </router-link>

    </div>`,
    methods: {
    },
};

const ListPosts = {
    data() {
        return {
            posts: { // todo: rename to articles
                published: [],
                draft: [],
            },
            loading_posts: true,
        };
    },
    template: `
    <div id="page-list-posts">
      <div
        style="background: black; color: white; overflow: hidden; position: fixed; top: 0; right: 0; left: 0;"
      >
        <div style="float: right; padding: 8px 16px;">
          Hola, {{ $user.nick }}
          <img :src="$user.picture" :alt="$user.nick" style="border-radius: 50%; height: 24px; vertical-align: middle;">
          &nbsp;
          <a class="btn btn-inv" href="/auth/logout">Salir</a>
        </div>
        <router-link 
          :to="{ name: 'home' }"
          style="color: white; text-decoration: none; float: left; padding: 12px 16px; font-weight: bold; font-size: 120%; font-family: 'source-serif-pro, Georgia, Cambria,Times, serif';"
        >
          GoPress
        </router-link>
      </div>
    
      <div class="workspace">
          <div>
            <h2>Entradas</h2>
            <p>Crea, edita y gestiona las entradas de tu sitio.</p>
          </div>
        
          <div class="post-type-filter">
            <router-link
              :to="{ name: 'listPosts', params: { filter: 'published' } }"
              :class="{ active: isFilter('published') }"
              class="nav"
            >
              Publicados
              <span class="counter">{{ posts.published.length }}</span>
            </router-link>
            <router-link
              :to="{ name: 'listPosts', params: { filter: 'draft' } }"
              :class="{ active: isFilter('draft') }"
              class="nav"
            >
              Borradores
              <span class="counter">{{ posts.draft.length }}</span>
            </router-link>
          </div>
          
          <div class="" style="overflow: hidden; padding: 8px 16px;">
            <div style="float: right;">
              <button class="btn btn-grad" @click="createArticle()">Añadir nueva entrada</button>
            </div>
            Entradas
          </div>

          <div class="list-posts">
            <div class="loader" v-if="loading_posts">Cargando posts...</div>
            <router-link 
              class="entry" 
              v-for="post in posts[$route.params.filter]" 
              :key="post.id"
              :to="{ name: 'editPost', params: { post_id: post.id } }"
            >
              <div class="entry-title">{{ post.title }}</div>
              <div>{{ post.created_on_pretty }}</div>
            </router-link>
          </div>
        
      </div>


    </div>`,
    computed: {
        // Método para verificar si el parámetro actual coincide
        isFilter() {
            return (filter) => this.$route.params.filter === filter;
        },
    },
    created() {
        this.fetchPosts();
    },
    methods: {
        fetchPosts() {
            let that = this;
            this.loading_posts = true;
            fetch(`/v1/articles`, {headers: fakeHeaders})
                .then(resp => resp.json())
                .then(function(list) {
                    list.forEach(post => {
                        post.created_on = new Date(post.created_on);
                        post.created_on_pretty = prettyDate(post.created_on);
                    });
                    list.sort((a,b) => compare(b.created_on, a.created_on));
                    that.loading_posts = false;
                    that.posts.published = list.filter(post => post.published === true);
                    that.posts.draft = list.filter(post => post.published === false);
                })
                .catch(function(e) {
                    that.loading_posts = false;
                });
        },
        createArticle() {
            const id = uuidv4();
            const body = {
                "id": id,
                "title": "",
            };
            let that = this;
            fetch('/v1/articles', {method: 'POST', body: JSON.stringify(body), headers: fakeHeaders})
                .then(resp => resp.json())
                .then(article => {
                    this.$router.push({ name: 'editPost', params: { post_id: article.id} });
                })
        },
    },

};

const EditPost = {
    data() {
        return {
            article: null,
            article_loading: true,
            saving: 0,
            publish_loading: false,
        };
    },
    template: `
    <div id="page-edit-post">
      <div
        style="background: black; color: white; overflow: hidden; position: fixed; top: 0; right: 0; left: 0; z-index: 9;"
      >
        <div
          v-if="article !== null" 
          style="float: right; padding: 8px 16px;"
        >
          <button
            class="btn btn-inv"
            :class="{'btn-grad': saving}"
            @click="save()"
          >
            <span v-if="saving == 0 && article.published">Guardar</span>
            <span v-else-if="saving == 0">Guardar como borrador</span>
            <span v-else-if="article.published">Guardando...</span>
            <span v-else>Guardando como borrador...</span>
          </button>&nbsp;
          <button
              v-if="!article.published" 
              class="btn btn-grad"
              @click="publishArticle()"
          >
            <span v-if="publish_loading">Publicando...</span>
            <span v-else>Publicar</span>
          </button>
        </div>
        <router-link 
          :to="{ name: 'home' }"
          style="color: white; text-decoration: none; float: left; padding: 12px 16px; font-weight: bold; font-size: 120%; font-family: 'source-serif-pro, Georgia, Cambria,Times, serif';"
        >
          GoPress
        </router-link>
        <div style="padding: 8px 16px;">
          <a 
            v-if="article !== null && article.published"
            class="btn btn-inv"
            :href="'/user/'+$user.nick+'/article/'+article.url" 
            target="_blank"
            style="font-size: 13px;"
          >Ver artículo</a>      
        </div>
      </div>
      
      <div class="loader" v-if="article_loading" style="padding: 80px 0;">Cargando artículo...</div>
      <div class="workspace" v-if="!article_loading">
          <div
            style="max-width: 650px; margin: 0 auto; padding-bottom: 50px; padding-top: 30px;"
          >          
              <input
                class="editor-title"
                type="text"
                placeholder="Añadir título"
                v-model="article.title"
                @keypress.enter="patchArticle({title: article.title})"
                @focusout="patchArticle({title: article.title})"
              >
          </div>
          <div id="editorjs"></div>
      </div>


    </div>`,
    created() {
        this.fetchArticle();
    },
    mounted() {
        window.addEventListener('keydown', this.handleKeydown);
    },
    beforeUnmount() {
        // Desregistrar el listener de teclado
        window.removeEventListener('keydown', this.handleKeydown);

        // TODO: Destruir la instancia de Editor.js si es necesario
        if (this.editor) {
            this.editor.destroy();
        }
    },
    methods: {
        fetchArticle() {
            let that = this;
            that.article_loading = true;
            let id = this.$route.params.post_id;
            return fetch('/v1/articles/'+encodeURIComponent(id), {headers: fakeHeaders})
                .then(resp => resp.json())
                .then(async article => {
                    // if (!article.tags) article.tags = [];
                    // article._tags = article.tags.join(", ")
                    that.article = article;
                    await sleep(500);
                    that.article_loading = false;
                    that.printArticle();
                })
                .catch(function () {
                    that.article_loading = false;
                });
        },
        patchArticle(params) {
            // todo: spinner saving

            // if (params.tags) {
            //     params.tags = this.selected._tags.split(",")
            //         .map(tag => tag.trim()) // todo: normalize case?
            //         .filter(tag => tag.length > 0)
            //         .sort(); // todo: sort by tag?
            // }

            this.saving++;
            let that = this;
            fetch('/v1/articles/'+encodeURIComponent(this.article.id), {method: 'PATCH', body: JSON.stringify(params), headers: fakeHeaders})
                .then(resp => resp.json())
                .then(async article => {
                    that.article.url = article.url;
                })
                .finally(async () => {
                    await sleep(500);
                    that.saving--;
                })
        },
        publishArticle() {
            if (!confirm("Are you sure to publish this article?")) return;
            let that = this;
            that.publish_loading = true;
            fetch('/v1/articles/'+encodeURIComponent(this.article.id)+"/publish", {method: 'POST', headers: fakeHeaders})
                .then(resp => resp.json())
                .then(async article => {
                    await sleep(2000);
                    that.article.published = true;
                    that.publish_loading = false;
                });
        },
        unpublishArticle() {
            if (!confirm("Are you sure to UNpublish this article?")) return;
            let that = this;
            fetch('/v1/articles/'+encodeURIComponent(this.article.id)+"/unpublish", {method: 'POST', headers: fakeHeaders})
                .then(resp => resp.json())
                .then(article => {
                    that.article.published = false;
                });
        },
        save() {
            let that = this;
            this.editor.save()
                .then((savedData) => {
                    that.patchArticle({content: {type:"editorjs", data:savedData}})
                })
                .catch((error) => {
                    console.error('Saving error', error);
                });
        },
        handleKeydown(event) {
            if ((event.ctrlKey || event.metaKey) && event.key === 's') {
                event.preventDefault(); // Previene el comportamiento predeterminado
                this.save(); // Llama a la función de guardar
            }
        },
        printArticle() {
            let that = this;
            this.editor = new EditorJS({

                placeholder: '¡Vamos a escribir una buena historia!',
                autofocus: true,

                /**
                 * Enable/Disable the read only mode
                 */
                readOnly: false,

                /**
                 * Wrapper of Editor
                 */
                holder: 'editorjs',

                /**
                 * Common Inline Toolbar settings
                 * - if true (or not specified), the order from 'tool' property will be used
                 * - if an array of tool names, this order will be used
                 */
                // inlineToolbar: ['link', 'marker', 'bold', 'italic'],
                // inlineToolbar: true,

                /**
                 * Tools list
                 */
                tools: {
                    /**
                     * Each Tool is a Plugin. Pass them via 'class' option with necessary settings {@link docs/tools.md}
                     */
                    header: {
                        class: Header,
                        inlineToolbar: ['marker', 'link'],
                        config: {
                            placeholder: 'Header'
                        },
                        shortcut: 'CMD+SHIFT+H'
                    },

                    /**
                     * Or pass class directly without any configuration
                     */
                    image: SimpleImage,

                    list: {
                        class: EditorjsList,
                        inlineToolbar: true,
                        shortcut: 'CMD+SHIFT+L'
                    },

                    checklist: {
                        class: Checklist,
                        inlineToolbar: true,
                    },

                    quote: {
                        class: Quote,
                        inlineToolbar: true,
                        config: {
                            quotePlaceholder: 'Enter a quote',
                            captionPlaceholder: 'Quote\'s author',
                        },
                        shortcut: 'CMD+SHIFT+O'
                    },

                    warning: Warning,

                    marker: {
                        class: Marker,
                        shortcut: 'CMD+SHIFT+M'
                    },

                    code: {
                        class: CodeTool,
                        shortcut: 'CMD+SHIFT+C'
                    },

                    delimiter: Delimiter,

                    inlineCode: {
                        class: InlineCode,
                        shortcut: 'CMD+SHIFT+C'
                    },

                    linkTool: {
                        class: LinkTool,
                        config: {
                            endpoint: '/v1/editor/helperFetchUrl', // Your backend endpoint for url data fetching,
                            headers: fakeHeaders,
                        }
                    },

                    embed: Embed,

                    table: {
                        class: Table,
                        inlineToolbar: true,
                        shortcut: 'CMD+ALT+T'
                    },

                    raw: RawTool,

                    underline: Underline,

                    image: {
                        class: ImageTool,
                        config: {
                            endpoints: {
                                byFile: '/v1/files', // Your backend file uploader endpoint
                                byUrl: 'http://localhost:9955/fetchUrl', // Your endpoint that provides uploading by Url
                            },
                            additionalRequestHeaders: fakeHeaders,
                        }
                    },

                    attaches: {
                        class: AttachesTool,
                        config: {
                            endpoint: '/v1/files',
                            additionalRequestHeaders: fakeHeaders,
                        }
                    },

                },

                /**
                 * This Tool will be used as default
                 */
                // defaultBlock: 'paragraph',

                /**
                 * Initial Editor data
                 */
                data: that.article.content.data,
                onReady: function () {
                },
                onChange: function (api, event) {
                    that.save();
                },
            });
        },
    },

};

// 2. Define some routes
// Each route should map to a component.
// We'll talk about nested routes later.
const routes = [
    {
        path: '/',
        name:'home',
        component: Home,
        redirect: { name: 'listPosts', params: { filter: 'published' } },
    },
    {
        path: '/posts/:filter',
        name: 'listPosts',
        component: ListPosts,
    },
    {
        path: '/edit/:post_id',
        name: 'editPost',
        component: EditPost,
    },
];

// 3. Create the router instance and pass the `routes` option
// You can pass in additional options here, but let's
// keep it simple for now.
const router = VueRouter.createRouter({
    // 4. Provide the history implementation to use. We are using the hash history for simplicity here.
    history: VueRouter.createWebHashHistory(),
    routes, // short for `routes: routes`
});

router.beforeEach(async (to, from, next) => {
    let that = this;
    //this.loading.databases = true;
    await fetch(`/auth/me`, {headers: fakeHeaders})
        .then(resp => resp.json())
        .then(function(user) {
            console.log(user);
            if (user.error) {
                window.location.href = '/auth/login';
                return;
            }
            app.config.globalProperties.$user = user;
            next();
        })
        .catch(function(e) {
            console.log('FAILED AUTH');
            window.location.href = '/auth/login';
        });
})

// 5. Create and mount the root instance.
const app = Vue.createApp({});
// Make sure to _use_ the router instance to make the
// whole app router-aware.
app.use(router);

app.mount('#app');

// 6. Utils


function pad(n, width, z) {
    z = z || '0';
    n = n + '';
    return n.length >= width ? n : new Array(width - n.length + 1).join(z) + n;
}

function prettyDate(d) {
    let now = new Date();

    let delta = (now.getTime() - d.getTime()) / 1000;

    if (delta < 60) {
        return 'Justo ahora';
    }

    if (delta < 3600) {
        return `Hace ${(delta/60).toFixed()} minutos`;
    }

    if (delta < 86400) {
        return `Hace ${(delta/3600).toFixed()} Horas`;
    }

    return `${d.getDate()}/${d.getMonth()+1}/${d.getFullYear()} a las ${d.getHours()}:${pad(d.getMinutes(),2)}`
}

function uuidv4() {
    return ([1e7]+-1e3+-4e3+-8e3+-1e11).replace(/[018]/g, c =>
        (c ^ crypto.getRandomValues(new Uint8Array(1))[0] & 15 >> c / 4).toString(16)
    );
}

function compare( a, b ) {
    if ( a < b ) return -1;
    if ( a > b ) return 1;
    return 0;
}

const sleep = (ms) =>
    new Promise(resolve => setTimeout(resolve, ms));
