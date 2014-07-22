// Copyright (c) 2004  Dustin Sallings <dustin@spy.net>

var lockedOpen = false;

// Toggle visibility of a named element
function toggleVisible(visible, element) {
	if(visible) {
		document.getElementById(element).style.display = "block";
	} else {
		document.getElementById(element).style.display = "none";
	}
}

// What to do when the mouse enters the olds div
function mouseEnteredOlds(event) {
	toggleVisible(true, "oldsul");
	toggleVisible(true, "oldstool");
}

// What to do when the mouse exits the olds div
function mouseExitedOlds(event) {
	if(!lockedOpen) {
		toggleVisible(false, "oldsul");
	}
	toggleVisible(false, "oldstool");
}

// What to do when the olds div is clicked
function clickedOlds(event) {
	var toolClass=null;
	if(lockedOpen) {
		lockedOpen = false;
		toolClass="unlocked";
	} else {
		lockedOpen = true;
		toolClass="locked";
	}
	var text=document.getElementById("oldstool");
	text.className=toolClass;
}

function loaded() {
	var oldsDiv=document.getElementById("olds");

	// Remove any items in the ul
	var oldsul=document.getElementById("oldsul");
	while(oldsul.hasChildNodes()) {
		oldsul.removeChild(oldsul.firstChild);
	}

	// Find every div that's a class of rssbox
	var divs = document.getElementsByTagName("div");
	var boxes = new Array();
	for(var i=0; i<divs.length; i++) {
		if(divs[i] != oldsDiv && divs[i].className == 'rssbox') {
			var uls=divs[i].getElementsByTagName("ul");
			if(uls.length == 1) {
				boxes.push(uls[0]);
			}
		}
	}

	// For every box we've found, copy the links into our new box.
	for(var i=0; i<boxes.length; i++) {
		var links = boxes[i].getElementsByTagName("a");
		for(var j=0; j<links.length; j++) {
			var myli = document.createElement("li");
			myli.appendChild(links[j].cloneNode(true));
			oldsul.appendChild(myli);
		}
	}

	// Turn off the display after it's built.  If we toggle it before, it never
	// wants to show up.
	toggleVisible(false, "oldsul");
}
