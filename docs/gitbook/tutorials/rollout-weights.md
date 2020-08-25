# Linkerd Rollout Weights

This guide shows you how to use weights in Flagger to have more fine-grained rollouts.  

By default Flagger allows to use linear promotion metrics, with the start value, the step and maximum weight value in 0 to 100 range.  

Example:
```yaml
canary:
  analysis:
    promotion:
      maxWeight: 50
      stepWeight: 20
```
This configuration performs analysis starting from 20, increasing by 20 until weight goes above 50.  
We would have steps (canary weight : primary weight):
* 20 (20 : 80)
* 40 (40 : 60)
* 60 (60 : 40)
* promotion

In order to enable non-linear promotion a new parameters were introduced:
* `fullWeight` - determines the sum of canary and primary weights, i.e. the maximum possible weight which can be set to the split
* `stepWeights` - determines the ordered array of weights, which shall be used during canary promotion.

Example:
```yaml
canary:
  analysis:
    promotion:
      fullWeight: 1000
      stepWeights: [1, 10, 100, 800]
```
This configuration performs analysis starting from 1, going through `stepWeights` values till 800.  
We would have steps (canary weight : primary weight):
* 1   (1 : 999)
* 10  (10 : 990)
* 100 (100 : 900)
* 800 (800 : 200)
* promotion
