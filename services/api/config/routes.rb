Server::Application.routes.draw do
  themes_for_rails

  # See http://guides.rubyonrails.org/routing.html

  namespace :arvados do
    namespace :v1 do
      resources :api_client_authorizations do
        post 'create_system_auth', on: :collection
      end
      resources :api_clients
      resources :authorized_keys
      resources :collections do
        get 'provenance', on: :member
        get 'used_by', on: :member
      end
      resources :groups do
        get 'owned_items', on: :member
      end
      resources :humans
      resources :job_tasks
      resources :jobs do
        get 'queue', on: :collection
        get 'log_tail_follow', on: :member
        post 'cancel', on: :member
      end
      resources :keep_disks do
        post 'ping', on: :collection
      end
      resources :links
      resources :logs
      resources :nodes do
        post 'ping', on: :member
      end
      resources :pipeline_instances
      resources :pipeline_templates
      resources :repositories do
        get 'get_all_permissions', on: :collection
      end
      resources :specimens
      resources :traits
      resources :user_agreements do
        get 'signatures', on: :collection
        post 'sign', on: :collection
      end
      resources :users do
        get 'current', on: :collection
        get 'system', on: :collection
        get 'event_stream', on: :member
        post 'activate', on: :member
        post 'setup', on: :collection
        post 'unsetup', on: :member
        get 'owned_items', on: :member
      end
      resources :virtual_machines do
        get 'logins', on: :member
        get 'get_all_logins', on: :collection
      end
    end
  end

  # omniauth
  match '/auth/:provider/callback', :to => 'user_sessions#create'
  match '/auth/failure', :to => 'user_sessions#failure'

  # Custom logout
  match '/login', :to => 'user_sessions#login'
  match '/logout', :to => 'user_sessions#logout'

  match '/discovery/v1/apis/arvados/v1/rest', :to => 'arvados/v1/schema#index'

  match '/static/login_failure', :to => 'static#login_failure', :as => :login_failure

  # Send unroutable requests to an arbitrary controller
  # (ends up at ApplicationController#render_not_found)
  match '*a', :to => 'static#render_not_found'

  root :to => 'static#home'
end
