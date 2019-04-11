package core

func (m Metadata) Less(m2 Metadata) bool {
	if m.Namespace == m2.Namespace {
		return m.Name < m2.Name
	}
	return m.Namespace < m2.Namespace
}

func (m Metadata) Ref() ResourceRef {
	return ResourceRef{
		Namespace: m.Namespace,
		Name:      m.Name,
	}
}

func (m Metadata) Match(ref ResourceRef) bool {
	return m.Namespace == ref.Namespace && m.Name == ref.Name
}
