# sprinkler

https://app.viam.com/module/erh/sprinkler

## instructions
* setup a raspberry pi with viam
* connect relays to gpio pins, 1 relay per sprinker zone
* add the board to your viam config
* add the sprinkler as a component and a config like this
```
{
  "lat": "40.5",
  "long": "-73.5",
  "zones": {
    "z1-front-garden": {
      "minutes": 10,
      "pin": "io16"
    },
    "z2-front-lawn-near-house": {
      "minutes": 10,
      "pin": "io17"
    },
    "z3-front-lawn-middle": {
      "pin": "io18",
      "minutes": 20
    },
    "z4-house-south-side": {
      "minutes": 20,
      "pin": "io25"
    }
  },
  "board": "local",
  "start_hour": 1,
  "data_dir": "/root/sprinkler_data/",
  "max_time_slice_minutes": 15
}
```
