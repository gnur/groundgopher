# groundgopher

A matrix style testing tool that allows you to run code for setup.  
It allows you to define multiple lists of test manipulators and validators and all permutations of these lists will be run.

If your API has 2 parameters (time and filter) and an auth header you create 3 lists of tests:

| auth    | time    | filter        |
| ------- | ------- | ------------- |
| none    | none    | none          |
| valid   | current | single param  |
| expired | +5m     | invalid param |
| invalid | -5m     | all params    |

And groundgopher will run 4 \* 4 \* 4 \* = 64 tests.

## glossary

### variable

A variable within groundgopher is a single parameter in the request that gets manipulated by a list of manipulators.  
In the table above `auth` is a variable, in this case a header.  
A variable is always a list of `case`s, but it is possible that a variable only has a single element

### case

A case is a single element in a variable list. A case consists of a `setup` function and a `validator` function.

### setup

The setup function manipulates the raw http request to make the required changes. For the `auth` variable of the example it will set a header (or not) to a certain value.

### validator

The validator will validate the raw http response. In the `expired` `case` above, it will verify that the response is a `403` and that the body contains an error message.  
It always returns a result

### result

A result always includes wether the request was valid and if it isn't, it also includes the reason.

### run

A run is the unique combination of a single case for every variable. It also includes the result and duration.

### report

The report contains all the runs and also the outcome (false if any run failed)

# acknowledgements

cartesion.go was heavily inspired (copied) from https://github.com/schwarmco/go-cartesian-product, thanks Joachim Swarm
