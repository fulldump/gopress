

// 1. Define route components.
// These can be imported from other files
const Home = {
    data() {
        return {
            databases: null,
            loading: {
                databases: false,
            },
        };
    },
    template: `
    <div id="page-home">
 
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
              :to="{ name: 'home', query: { filter: ''} }"
              class="nav"
              >
                  Publicados
                  <span class="counter">29</span>
              </router-link>
            <router-link
              :to="{ name: 'home', query: { filter: 'draft'} }"
              class="nav"
              >
                  Borradores
                  <span class="counter">3</span>
              </router-link>
          </div>
        
          <div
            v-if="loading.databases"
            style="text-align: center;"
          >
            <div class="loader">Loading databases...</div>
          </div>
    
          <div class="buttons">
            <router-link
              class="button button-blue"
              to="/published"
          >Publicados</router-link>
          
            <router-link
              :to="{ name: 'home', query: { filter: 'draft'} }"
              class="nav"
              to="/drafts"
          >Borradores</router-link>
          </div>
      </div>
    
    </div>`,
    computed: {
        username() {
            // We will see what `params` is shortly
            return this.$route.params.username
        },
    },
    created() {
        this.fetchDatabases();
    },
    methods: {
        fetchDatabases() {
            let that = this;
            this.loading.databases = true;
            fetch(`/v1/databases`, {headers: fakeHeaders})
                .then(resp => resp.json())
                .then(function(list) {
                    that.loading.databases = false;
                    list.sort((a,b) => compare(a.name, b.name));
                    that.databases = list;
                })
                .catch(function(e) {
                    that.loading.databases = false;
                });
        },
    },
};

const ListPosts = {
    data() {
        return {
            posts: [],
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
            <div class="entry"  v-for="post in posts" :key="post.id" v-if="post.published">
              <div v-if="isFilter('published') && post.published || isFilter('draft') && post.published == false " >
                            <div>{{ post.title }}</div>
              <div>{ { post.timestamp } }</div>
              {{ post }}
              </div>
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
                    that.posts = list;
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
    { path: '/', name:'home', component: Home },
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


