App.PerformActionMixin = Ember.Mixin.create
  performAction: (method, action, tempState, data=null) ->
    record = @get('model')
    prevState = record.get('state')
    record.set 'state', tempState
    rootKey = record.constructor.rootKey
    ajax = record.ajax(method, action, data)
    ajax.then (json) ->
      if attrs = json[rootKey]
        record.load(attrs)
    , (jqxhr) -> # record.ajax
      defaultErrMsg = if action
        "Failed to #{action}!"
      else
        "Failed!"
      App.postError(jqxhr.responseJSON, defaultErrMsg)
      record.set('state', prevState)
    ajax
