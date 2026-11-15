package scene

// Bool creates root-owned component state and optionally connects it to Saves.
// persistent IDs must be durable author-supplied IDs, not render entity numbers.
func Bool(root *Root, saves *Saves, identity string, initial, persistent bool) (*Value[bool], error) {
	if err := root.Claim(identity); err != nil {
		return nil, err
	}

	value := NewValue(initial)

	value.owner = root
	if persistent {
		if err := Persist(root, saves, identity, 1, value.Get, func(bool) error { return nil }, value.Set); err != nil {
			return nil, err
		}
	}

	return value, nil
}
