

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
        style="background: black; color: white; padding: 16px;"
      >
        Barra del usuario
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
            <div class="entry">
              <div style="float: right;">
                Añadir nueva entrada
              </div>
              Entradas
            </div>
            <div class="entry" v-for="post in posts[$route.params.filter]" :key="post.id">
              <div class="entry-title">{{ post.title }}</div>
              <div>{ { post.timestamp } }</div>
            </div>
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
    // { path: '/databases/:database_id', name: 'database', component: Database},
    // { path: '/databases/:database_id/create-collection', name: 'create-collection', component: CollectionCreate},
    // { path: '/databases/:database_id/collections/:collection_name', name: 'collection', component: Collection},
    // { path: '/databases/:database_id/collections/:collection_name/delete', name: 'delete-collection', component: CollectionDelete},
    // { path: '/databases/:database_id/collections/:collection_name/indexes', name: 'indexes', component: CollectionIndexes},
    // { path: '/databases/:database_id/collections/:collection_name/defaults', name: 'defaults', component: CollectionDefaults},
    // { path: '/databases/:database_id/delete', name: 'delete-database', component: DatabaseDelete},
];

// 3. Create the router instance and pass the `routes` option
// You can pass in additional options here, but let's
// keep it simple for now.
const router = VueRouter.createRouter({
    // 4. Provide the history implementation to use. We are using the hash history for simplicity here.
    history: VueRouter.createWebHashHistory(),
    routes, // short for `routes: routes`
});

// 5. Create and mount the root instance.
const app = Vue.createApp({});
// Make sure to _use_ the router instance to make the
// whole app router-aware.
app.use(router);

app.mount('#app');


