package main

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/graphql-go/graphql"
	"golang.org/x/exp/constraints"
)

var typeTime = reflect.TypeOf(time.Time{})

type Pair[T1 any, T2 any] struct {
	First  T1
	Second T2
}

func executeQuery(query string, schema graphql.Schema) (*graphql.Result, error) {
	result := graphql.Do(graphql.Params{
		Schema:        schema,
		RequestString: query,
	})
	if len(result.Errors) > 0 {
		return nil, result.Errors[0].OriginalError()
	}

	return result, nil
}

func getBasicOutput(t reflect.Type) graphql.Output {
	kind := t.Kind()
	switch kind {
	case reflect.String:
		return graphql.String

	case reflect.Int, reflect.Uint,
		reflect.Int8, reflect.Uint8,
		reflect.Int16, reflect.Uint16,
		reflect.Int32, reflect.Uint32:
		// Use Float since the GraphQL implementation is limited to 32-Bit.
		// The resolver functions will cast the integer to float64 and return it.
		// https://github.com/graphql/graphql-spec/issues/73
		return graphql.Float

	case reflect.Int64, reflect.Uint64:
		return graphql.Float

	case reflect.Bool:
		return graphql.Boolean

	case reflect.Float32, reflect.Float64:
		return graphql.Float
	default:
		return nil
	}
}

// Browses through the members of a given type and creates
// the corresponding field and output structure of given type.
// As an oversimplification, it works similarly to json.Marshal
// but for GraphQL.
func createGraphQlFieldHierarchy(t reflect.Type, typesMap map[string]Pair[graphql.Output, graphql.Fields], filterMap map[string]graphql.ArgumentConfig) (graphql.Output, graphql.Fields) {

	// GraphQL complains when a type with the same name is registered once. Error:
	// Schema must contain uniquely named types but contains multiple types named "XYZ".
	//
	// This can happen if a struct has two member that are of the same type:
	// type Foo struct {
	//		X FooStruct <-- At first object type for 'FooStruct' is generated ...
	//		Y FooStruct <-- ... then 'FooStruct' is attempted to be generated again.
	// }
	//
	// To avoid the error above we keep a map of all registered types and return this on a match.
	if typesMap == nil {
		typesMap = make(map[string]Pair[graphql.Output, graphql.Fields], 0)
	}

	if filterMap == nil {
		filterMap = make(map[string]graphql.ArgumentConfig, 0)
	}

	knownType, ok := typesMap[t.Name()]
	if ok {
		return knownType.First, knownType.Second
	}

	// The code automatically transforms some types, such as time.Time, because their structure is unnecessarily complex
	// for GraphQL output. For instance, the 'loc' in time.Time isn't needed and the type can be a simple timestamp.
	switch t {
	case typeTime:
		// Return float due to the 32-bit limitations of ints
		return graphql.Float, nil
	}

	switch t.Kind() {
	case reflect.Func:
		// Retrieve the return type of the function
		returnType := t.Out(0)
		if returnType.Kind() == reflect.Interface {
			// Return type must be explicitly defined, no interface/any allowed
			// as the type is used to generate the GraphQL schema.
			return nil, nil
		}

		structFieldType, fields := createGraphQlFieldHierarchy(returnType, typesMap, filterMap)
		return structFieldType, fields
	case reflect.Struct:

		fields := graphql.Fields{}

		for _, structField := range reflect.VisibleFields(t) {
			// Subfields are fields from struct subtypes.
			// E.g:
			// type Bar struct {
			//   	X string <-- field 1
			//		Y string <-- field 2
			// }
			//
			// type Foo struct {
			//    B []Bar
			// }
			//
			// 'structField' is 'B', subfields are 'X' and 'Y'.
			// They are needed to register filter resolvers for arrays/slices.
			// foos(X: "abc") {
			//		X
			// }
			//
			structFieldType, subfields := createGraphQlFieldHierarchy(structField.Type, typesMap, filterMap)

			// Skip unsupported types
			if structFieldType == nil {
				continue
			}

			args := graphql.FieldConfigArgument{}

			// Register all filter arguments
			for k, v := range subfields {
				switch v.Type {
				case graphql.String, graphql.Int, graphql.Boolean, graphql.Float:
					args[k] = &graphql.ArgumentConfig{
						Type: v.Type,
					}
				}
			}

			// Value copy to ensure proper capturing of variable in Resolve closure.
			// https://eli.thegreenplace.net/2019/go-internals-capturing-loop-variables-in-closures/
			structFieldName := structField.Name
			structFieldTypeKind := structField.Type.Kind()

			switch structFieldTypeKind {
			// Add helper paramters to graphql lists
			case reflect.Slice, reflect.Array:

				// Add where filter if the array or slice contains structs
				if structField.Type.Elem().Kind() == reflect.Struct {

					// Example syntax:
					// items (where: {X: "abc"}) { X }
					// If the filter object contains multiple fields,
					// the filter will be applied as an OR operation.
					// Meaning, the first item that matches any value
					// of the fields will be returned.

					argConfig, ok := filterMap[structFieldName]
					if !ok {
						fields := graphql.InputObjectConfigFieldMap{}

						for _, v := range reflect.VisibleFields(structField.Type.Elem()) {
							t := getBasicOutput(v.Type)
							if t != nil {
								fields[strings.ToLower(v.Name)] = &graphql.InputObjectFieldConfig{
									Type: t,
								}
							}
						}

						argConfig = graphql.ArgumentConfig{
							Type: graphql.NewInputObject(graphql.InputObjectConfig{
								Name:   strings.ToLower(structFieldName),
								Fields: fields,
							}),
						}

						filterMap[structFieldName] = argConfig
						ok = true
					}
					if ok {
						args["where"] = &argConfig
					}
				} else {
					// Add skip filter
					args["skip"] = &graphql.ArgumentConfig{
						Type: graphql.Int,
					}

					// Add limit filter
					args["limit"] = &graphql.ArgumentConfig{
						Type: graphql.Int,
					}
				}
			}

			fields[strings.ToLower(structFieldName)] = &graphql.Field{
				Name: structField.Name,
				Type: structFieldType,
				Args: args,
				Resolve: func(p graphql.ResolveParams) (any, error) {
					r := reflect.ValueOf(p.Source).FieldByName(structFieldName)
					switch structFieldTypeKind {
					case reflect.Func:
						// Return 'null' if function field is nil
						if r.IsNil() {
							return nil, nil
						}

						// Function siganture is func() T
						var err error
						results := reflect.ValueOf(r.Interface()).Call([]reflect.Value{reflect.ValueOf(p.Source)})
						if results[1].Interface() != nil {
							err = results[1].Interface().(error)
						}
						return results[0].Interface(), err
					case reflect.Slice, reflect.Array:

						i := 0
						j := r.Len()

						// Evaluate the 'where' argument
						filterOne, filterOneSet := p.Args["where"]
						if filterOneSet {
							for i := 0; i < j; i++ {
								element := r.Index(i)

								filter := filterOne.(map[string]interface{})
								for fieldName, filterValue := range filter {

									val := element.FieldByNameFunc(func(s string) bool {
										return strings.ToLower(s) == fieldName
									})

									var match bool
									// If the filter value is a number, then it is of type float64
									// due to graphql.Float. See getBasicOutput.
									switch filterValue.(type) {
									case float64:
										fv := filterValue.(float64)
										rv := val.Interface()
										if yFloat64, ok := rv.(float64); ok {
											match = fv == yFloat64
										} else if yInt, ok := rv.(int); ok {
											match = fv == float64(yInt)
										} else if yInt8, ok := rv.(int8); ok {
											match = fv == float64(yInt8)
										} else if yInt32, ok := rv.(int32); ok {
											match = fv == float64(yInt32)
										} else if yInt64, ok := rv.(int64); ok {
											match = fv == float64(yInt64)
										}
									default:
										match = filterValue == val.Interface()
									}

									if match {
										return r.Slice(i, i+1).Interface(), nil
									}
								}

							}
							// item with path not found
							return r.Slice(0, 0).Interface(), nil
						}

						// Evaluate the 'skip' argument
						skip, skipOk := p.Args["skip"]
						if skipOk {
							i = Min(skip.(int), j-1)
						}

						// Evaluate the 'limit' argument
						limit, limitOk := p.Args["limit"]
						if limitOk {
							j = Min(i+limit.(int), j)
						}

						return r.Slice(i, j).Interface(), nil
					}

					// remove r.Kind()?
					switch r.Kind() {
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						// Use float since graphql int is limited to 32-bit.
						// Check getBasicOutput() for more info.
						return float64(r.Int()), nil

					case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
						// Use float since graphql int is limited to 32-bit.
						// Check getBasicOutput() for more info.
						return float64(r.Uint()), nil

					case reflect.Bool:
						return r.Bool(), nil

					case reflect.Float32, reflect.Float64:
						return r.Float(), nil
					case reflect.String:
						return r.Interface(), nil
					case reflect.Struct:

						switch r.Type() {
						case typeTime:
							t := r.Interface().(time.Time).UnixMilli()
							return float64(t), nil
						}

						return r.Interface(), nil
					}

					return nil, errors.New("unknown type")
				},
			}
		}

		o := graphql.NewObject(graphql.ObjectConfig{
			Name:   t.Name(),
			Fields: fields,
		})

		typesMap[t.Name()] = Pair[graphql.Output, graphql.Fields]{First: o, Second: fields}

		return o, fields
	case reflect.Array, reflect.Slice:
		nt, fields := createGraphQlFieldHierarchy(t.Elem(), typesMap, filterMap)
		return graphql.NewList(nt), fields
	default:
		return getBasicOutput(t), nil
	}
}

func QueryStructViaGraphql[T any](rootField string, o T, query string) ([]byte, error) {
	typ, _ := createGraphQlFieldHierarchy(reflect.TypeOf(o), nil, nil)
	fields := graphql.Fields{}
	fields[rootField] = &graphql.Field{
		Type: typ,
		Resolve: func(p graphql.ResolveParams) (any, error) {
			return o, nil
		},
	}

	rootQuery := graphql.ObjectConfig{Name: "RootQuery", Fields: fields}
	schemaConfig := graphql.SchemaConfig{Query: graphql.NewObject(rootQuery)}
	schema, err := graphql.NewSchema(schemaConfig)
	if err != nil {
		return nil, err
	}

	result, err := executeQuery(query, schema)
	if err != nil {
		return nil, err
	}

	b, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Returns the minimum of the two given objects.
func Min[T constraints.Integer](x, y T) T {
	if x < y {
		return x
	}
	return y
}
