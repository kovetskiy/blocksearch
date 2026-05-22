// CUE: a schema definition. Searching the field returns its whole struct.
package billing

#Invoice {
	name:    string
	amount:  int
	paid:    bool
	payment: null | string
}
