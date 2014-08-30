Rails.application.routes.draw do
  mount GoAppProxy.new => '/api/v0'
  get '/home' => 'pages#home'
  get '/kit'  => 'pages#kit'
  root 'web_app#index'
end
