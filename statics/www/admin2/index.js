

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
            posts: {
                published: [],
                draft: [],
            },
        };
    },
    template: `
    <div id="page-list-posts">
      <div
        style="background: black; color: white; overflow: hidden;"
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
            <p>Crea, edita y gestiona las entradas de tu sitio. Más información.</p>
          </div>
        
          <div class="post-type-filter">
            <router-link
              :to="{ name: 'listPosts', params: { filter: 'published' } }"
              :class="{ active: isFilter('published') }"
              class="nav"
            >
              Publicados
              <span class="counter">29</span>
            </router-link>
            <router-link
              :to="{ name: 'listPosts', params: { filter: 'draft' } }"
              :class="{ active: isFilter('draft') }"
              class="nav"
            >
              Borradores
              <span class="counter">3</span>
            </router-link>
          </div>
          
          <div class="list-posts">
            <div class="" style="overflow: hidden; padding: 8px 16px;">
              <div style="float: right;">
                <button class="btn btn-grad">Añadir nueva entrada</button>
              </div>
              Entradas
            </div>
            <router-link 
              class="entry" 
              v-for="post in posts[$route.params.filter]" 
              :key="post.id"
              :to="{ name: 'editPost', params: { post_id: post.id } }"
            >
              <div class="entry-title">{{ post.title }}</div>
              <div>{ { post.timestamp } }</div>
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
            //this.loading.databases = true;
            fetch(`/v1/articles`, {headers: fakeHeaders})
                .then(resp => resp.json())
                .then(function(list) {

                    //that.loading.databases = false;
                    //list.sort((a,b) => compare(a.name, b.name));
                    that.posts.published = list.filter(post => post.published === true);
                    that.posts.draft = list.filter(post => post.published === false);
                })
                .catch(function(e) {
                    //that.loading.databases = false;
                });
        },
    },

};


const EditPost = {
    data() {
        return {};
    },
    template: `
    <div id="page-edit-post">
      <div
        style="background: black; color: white; overflow: hidden;"
      >
        <div style="float: right; padding: 8px 16px;">
          <button class="btn btn-inv">Guardar como borrador</button>
          <button class="btn btn-grad">Publicar</button>
        </div>
        <router-link 
          :to="{ name: 'home' }"
          style="color: white; text-decoration: none; float: left; padding: 12px 16px; font-weight: bold; font-size: 120%; font-family: 'source-serif-pro, Georgia, Cambria,Times, serif';"
        >
          GoPress
        </router-link>
        <div style="padding: 16px;">
          {{ $user.nick }}      
        </div>
      </div>
    
      <div class="workspace">
            sdfafd
      </div>


    </div>`,
    created() {
    },
    methods: {
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


