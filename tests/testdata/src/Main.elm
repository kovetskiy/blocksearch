-- Elm: searching the function name returns the whole declaration.
module Main exposing (greet)


greet : String -> String
greet name =
    "Hello, " ++ name ++ "!"


main =
    greet "billing"
