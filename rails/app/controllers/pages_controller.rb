class PagesController < ApplicationController
  def home
  end

  def kit
  end

  def about
  end

  def team
    @members = YAML.load(File.read(File.join(Rails.root, 'config', 'team.yml'))).shuffle
  end
end
