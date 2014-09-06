Rails.application.routes.draw do
  mount GoAppProxy.new => '/api/v0'
  get '/home'   => 'pages#home'
  get '/kit'    => 'pages#kit'
  get '/about'  => 'pages#about'
  get '/team'   => 'pages#team'
  get '/vpc'    => 'pages#vpc'
  root 'web_app#index'
end
