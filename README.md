# GraphQL query API for Go structs

## Overview

<img alt="GraphQL Logo" align="right" src="https://graphql.org/img/logo.svg" width="15%" />

This API provides a simple and efficient way to query specific fields from Go structs.

## Key Features

1. **Selective Field Queries**: Query only the fields you need.
2. **Nested Struct Support**: Ability to query fields within nested structs.
3. **Easy Integration**: Works seamlessly with popular Go frameworks like Echo, also check out the examples.

## Quick Start

1. **Define Your Structs**:

    Example:

    ```go
    type Cat struct {
        Name string `json:"name"`
        Age  int    `json:"age"`
        Color string `json:"color"`
    }
    ```

2. **Use `QueryStructViaGraphql` Function**:

    This function helps in querying a struct based on the provided GraphQL query string.

    Example:

    ```go
    b, err := QueryStructViaGraphql("cats", cats, post.Query);
    ```

3. **Set Up Routes**:

    Use your favorite framework (like Echo) to set up routes and handle requests.

    Example:

    ```go
    e.POST("/dogs", QueryDogs)
    e.POST("/cats", QueryCats)
    ```

4. **Run Your Server**:

    ```go
    e.Logger.Fatal(e.Start(":8000"))
    ```

## Usage

Make a POST request to `/cats` or `/dogs` with a request body containing your GraphQL query.

Example Request Body:

```graphql
{
    cats
    {
        name
        age
    }
}
```

Response will contain only the `name` and `age` fields for the respective struct. A [Postman](https://www.postman.com/) example file called `postman_examples_import_me.json` is included in the repository. Start the Go server via `go run .` and import the json file into Postman to try out the examples.

## License

MIT License. See [LICENSE](LICENSE.md) for more information.