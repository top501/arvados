class Group < ArvadosBase
  def self.goes_in_projects?
    true
  end

  def self.copies_to_projects?
    false
  end

  def self.contents params={}
    res = arvados_api_client.api self, "/contents", {
      _method: 'GET'
    }.merge(params)
    ret = ArvadosResourceList.new
    ret.results = arvados_api_client.unpack_api_response(res)
    ret
  end

  def contents params={}
    res = arvados_api_client.api self.class, "/#{self.uuid}/contents", {
      _method: 'GET'
    }.merge(params)
    ret = ArvadosResourceList.new
    ret.results = arvados_api_client.unpack_api_response(res)
    ret
  end

  def class_for_display
    group_class == 'project' ? 'Project' : super
  end

  def textile_attributes
    [ 'description' ]
  end

  def self.creatable?
    false
  end
end
