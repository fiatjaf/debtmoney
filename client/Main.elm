import Html exposing
  ( Html, text
  , h1, h2, div, textarea, button, p, a
  , table, tbody, thead, tr, th, td
  , input, select, option, header, nav
  , span, section, nav, img, label
  )
import Html.Attributes exposing (..)
import Html.Events exposing (onClick, onInput, onSubmit, onWithOptions)
import Navigation exposing (Location)
import Task exposing (Task)
import Http
import Dict
import Time exposing (Time)
import Process
import Json.Decode as JD
import Json.Encode as JE
import Result
import Date
import Date.Format
import GraphQL.Client.Http exposing (sendQuery, sendMutation)
import GraphQL.Request.Builder exposing (request)

import Page exposing (..)
import User exposing (..)
import Thing exposing (..)
import Helpers exposing (..)


type alias Flags = {}


main =
  Navigation.programWithFlags
    (.pathname >> Navigate)
    { init = init
    , view = view
    , update = update
    , subscriptions = subscriptions
    }


-- MODEL
type alias Model =
  { me : User.User
  , route : Page
  , user : User.User
  , thing : Thing.Thing
  , error : String
  , loading : String
  }


init : Flags -> Location -> (Model, Cmd Msg)
init flags location =
  let 
    (m, loadmyself) = update LoadMyself
      <| Model
        User.defaultUser
        HomePage
        User.defaultUser
        Thing.defaultThing
        ""
        ""
    (nextm, handlelocation) = update (Navigate location.pathname) m
  in
    nextm ! [ loadmyself, handlelocation ]


-- UPDATE


type Msg
  = EraseError
  | Navigate String
  | LoadMyself
  | GotMyself (Result GraphQL.Client.Http.Error User.User)
  | GotUser (Result GraphQL.Client.Http.Error User.User)
  | GotThing (Result GraphQL.Client.Http.Error Thing)
  -- | ChangeDebtCreditor String
  -- | ChangeDebtAsset String
  -- | ChangeDebtAmount String
  -- | SubmitDebtDeclaration
  -- | GotDebtDeclarationResponse (Result Http.Error ServerResult)
  -- | ConfirmThing Int
  -- | GotThingConfirmationResponse (Result Http.Error ServerResult)

update : Msg -> Model -> (Model, Cmd Msg)
update msg model =
  case msg of
    EraseError ->
      ( { model | error = "", loading = "" }
      , Cmd.none
      )
    Navigate pathname ->
      let
        route = match pathname
        m = { model | route = route }
        (nextm, effect) = case route of
          HomePage -> update LoadMyself m
          ThingPage thingId ->
            ( { m | loading = "Loading thing..." }
            , request thingId thingQuery
              |> sendQuery "/_graphql"
              |> Task.attempt GotThing
            )
          UserPage userId ->
            ( { m | loading = "Loading " ++ userId ++ "'s profile..." }
            , request userId userQuery
              |> sendQuery "/_graphql"
              |> Task.attempt GotUser
            )
          NotFound -> ( m, Cmd.none)
        updateurl = if route == model.route
          then Cmd.none
          else Navigation.newUrl pathname
      in
        nextm ! [ effect, updateurl ]
          
    LoadMyself ->
      ( { model | loading = "Loading your profile..." }
      , request "me" userQuery
        |> sendQuery "/_graphql"
        |> Task.attempt GotMyself
      )
    GotMyself result ->
      case result of
        Ok user -> { model | me = user, loading = "" } ! []
        Err err ->
          ( { model | error = errorFormat err }
          , delay (Time.second * 4) EraseError
          )
    GotUser result ->
      case result of
        Ok user -> { model | user = user, loading = "" } ! []
        Err err ->
          ( { model | error = errorFormat err }
          , delay (Time.second * 4) EraseError
          )
    GotThing result ->
      case result of
        Ok thing -> { model | thing = thing, loading = "" } ! []
        Err err ->
          ( { model | error = errorFormat err }
          , delay (Time.second * 4) EraseError
          )
    -- ChangeDebtCreditor x ->
    --   { model | declaringDebt = model.declaringDebt |> Thing.setCreditor x } ! []
    -- ChangeDebtAsset x ->
    --   { model | declaringDebt = model.declaringDebt |> Thing.setAsset x } ! []
    -- ChangeDebtAmount x ->
    --   { model | declaringDebt = model.declaringDebt |> Thing.setAmount x } ! []
    -- SubmitDebtDeclaration ->
    --   ( { model | loading = "Submitting debt declaration..." }
    --   , submitDebt model GotDebtDeclarationResponse
    --   )
    -- GotDebtDeclarationResponse result ->
    --   case result of
    --     Ok thing -> update LoadMyself { model | loading = "" }
    --     Err err ->
    --       ( { model | error = errorFormat err }
    --       , delay (Time.second * 4) EraseError
    --       )
    -- ConfirmThing thingId ->
    --   ( model
    --   , submitConfirmation thingId GotThingConfirmationResponse
    --   )
    -- GotThingConfirmationResponse result ->
    --   case result of
    --     Ok thing -> update LoadMyself { model | loading = "" }
    --     Err err ->
    --       ( { model | error = errorFormat err }
    --       , delay (Time.second * 4) EraseError
    --       )


-- SUBSCRIPTIONS
subscriptions : Model -> Sub Msg
subscriptions model =
  Sub.none


-- VIEW
view : Model -> Html Msg
view model =
  div []
    [ nav [ class "navbar" ]
      [ div [ class "navbar-brand" ]
        [ div [ class "navbar-item logo" ] [ text "debtmoney" ]
        , div [ class "navbar-item" ]
          [ div [ class "field" ]
            [ if model.me.id == ""
              then a [ href "/" ] [ text "login" ]
              else text model.me.id
            ]
          ]
        ]
      ]
    , div []
      [ if model.error /= ""
        then div [ id "error", class "notification is-danger" ] [ text <| model.error ]
        else if model.loading /= ""
        then div [ id "loading", class "pageloader" ]
          [ div [ class "spinner" ] []
          , div [ class "title" ] [ text <| model.loading ]
          ]
        else div [] []
      ]
    , section [ class "section" ]
      [ case model.route of
        HomePage -> userView model.me
        ThingPage r -> thingView model.thing
        UserPage u -> userView model.user
        NotFound -> div [] [ text "this page doesn't exists" ]
      ]
    ]


userView : User -> Html Msg
userView user =
  div [ id "user" ]
    [ h1 []
      [ text <| user.id ++ "'s" ++ " profile"
      ]
    , div []
      [ h2 [] [ text "operations:" ]
      , table [ class "table is-hoverable is-fullwidth" ]
        [ thead []
          [ tr []
            [ th [] [ text "date" ]
            , th [] [ text "description" ]
            , th [] [ text "confirmed" ]
            ]
          ]
        , tbody [] []
          -- <| List.map (thingRow user.id) user.things
        ]
      ]
    , div []
      [ h2 [] [ text "address:" ]
      , p [] [ text user.address]
      ]
    , div []
      [ h2 [] [ text "balances:" ]
      , table [ class "table is-striped is-fullwidth" ]
        [ thead []
          [ tr []
            [ th [] [ text "asset" ]
            , th [] [ text "amount" ]
            , th [] [ text "trust limit" ]
            ]
          ]
        , tbody []
          <| List.map assetRow user.balances
        ]
      ]
    ]

--     , if itsme then div [ id "declaringdebt" ]
--       [ h2 [] [ text "Declare a debt:" ]
--       , formField "Creditor:"
--         <| input
--           [ type_ "text"
--           , class "input"
--           , placeholder "name@gmail.com"
--           , onInput ChangeDebtCreditor
--           ] []
--       , formField "Currency:"
--         <| input
--           [ type_ "text"
--           , class "input"
--           , placeholder "USD"
--           , onInput ChangeDebtAsset
--           ] []
--       , formField "Amount"
--         <| input
--           [ type_ "text"
--           , class "input"
--           , placeholder "37"
--           , onInput ChangeDebtAmount
--           ] []
--       , formField ""
--         <| button
--           [ onClick SubmitDebtDeclaration
--           , class "button is-primary"
--           ] [ text "submit" ]
--       ]
--       else text ""
--     ]
-- 
-- formField : String -> Html Msg -> Html Msg
-- formField labeltext inputelem =
--   div [ class "field is-horizontal" ]
--     [ div [ class "field-label is-normal" ] [ label [ class "label" ] [ text labeltext ] ]
--     , div [ class "field-body" ]
--       [ div [ class "field is-narrow" ]
--         [ div [ class "control" ] [ inputelem ]
--         ]
--       ]
--     ]
--   
-- 
-- thingRow : Bool -> String -> Thing.Thing -> Html Msg
-- thingRow itsme userId thing =
--   let 
--     confirm =
--       if itsme
--         then if List.member userId thing.confirmed
--         then text ""
--         else button [ onClick <| ConfirmThing thing.id ] [ text "confirm" ]
--       else text ""
--   in
--     tr []
--       [ td [] [ link ("/thing/" ++ (toString thing.id)) (date thing.date) ]
--       , td [] [ thingDescription thing ]
--       , td []
--         [ table []
--           [ tr []
--             <| confirm ::
--               List.map
--                 (td [] << List.singleton << userLink)
--                 thing.confirmed
--           ]
--         ]
--       ]
-- 
-- thingDescription
--       span []
--         [ span []
--             <| List.map userLink
--             <| Dict.keys bs.parties
--         , text " have paid "
--         , span []
--             <| List.map (\p -> text <| p.paid ++ " of " ++ p.due ++ " due")
--             <| Dict.values bs.parties
--         , text " for "
--         , text bs.object
--         ]

assetRow : Balance -> Html Msg
assetRow balance =
  tr []
    [ td [] [ text balance.asset ]
    , td [] [ text balance.amount ]
    , td [] [ text balance.limit ]
    ]
