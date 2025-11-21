go build goalaric.go 
godepgraph -s Goalaric > godep.x 
dot godep.x -Tpng -o godepgraph.png 
godepgraph.png