MapUp Full-Stack Assignment: Prep Guide
Geofencing & Real-time Alert System. Read this alongside the assignment brief.

This guide shows what reviewers check and what MapUp asks in the rounds after the assignment. It has no code on purpose: your submission must be your own work, and similar submissions get both candidates disqualified. You will also be asked to explain every part of what you built.
1. Easy-to-miss requirements
Exact endpoints. Paths, methods and field names must match the brief. Evaluation is strict on this.
time_ns on every response. Measure it inside each handler, including error responses.
Geofence validation. Closed polygon (first point equals last), at least 4 points, valid latitude and longitude ranges, and one of the 4 allowed categories. Return a clear 400 with a message when input is invalid.
Coordinate order. The brief uses [latitude, longitude]. PostGIS and GeoJSON use [longitude, latitude]. Mixing these up is the most common bug.
Entry and exit are changes, not states. An event fires only when a vehicle moves from outside to inside a geofence, or the reverse. Compare with the vehicle's previous position.
Alerts must not slow the API. Deliver them without blocking the location update response, and store every alert in the database.
Submission checklist.
Private repo with all collaborators added
SETUP.md
Backend image on Docker Hub
Backend and frontend both deployed
Video link in the README
Google Form submitted.
2. Concepts to be clear on
Topic
What to understand
Point-in-polygon
How the ray-casting idea works, and what PostGIS functions such as ST_Contains and ST_Covers do. Decide how you treat a point exactly on the boundary and write that decision down.
Spatial index
What a GiST index on a geometry column does and why containment checks stay fast as geofences grow.
Entry and exit detection
Keep the set of geofences each vehicle was last inside. On a new location, compare old and new sets: new ones are entries, missing ones are exits. Handle a vehicle's very first update.
Alert rules
A rule can target one vehicle or all vehicles, for entry, exit or both. Be able to explain how you match an event to rules.
WebSocket delivery
The "hub" pattern: one place tracks connected clients and broadcasts to them. Know how you avoid race conditions on the client list, drop dead connections, and reconnect from the frontend.
Go concurrency
Goroutines, channels, mutexes, and how to check for race conditions with the race detector.
Docker
Multi-stage builds for a small image, Docker Compose with the database and API, configuration through environment variables.
Deployment
CORS between frontend and backend, using wss:// when the site is on HTTPS, and setting the API URL through environment variables.
React
Managing a live list of alerts in state, loading and error states, and avoiding unnecessary re-renders with useMemo, useCallback and React.memo. A map library such as Leaflet covers polygons, markers and map clicks.


3. Suggested build order
Database schema, then the geofence and vehicle endpoints with validation.
Location updates: containment check, entry and exit detection, history storage.
Alert rules and real-time delivery.
Frontend: map, forms, live alert feed, history with filters and pagination.
Docker, deployment and SETUP.md.
Video.
Test each step before moving on. A smaller system that works end to end scores better than a larger one that half works.
4. The video (30 to 45 minutes)
Demo, 10 to 12 min. Create a geofence, register a vehicle, move it in and out, and show the alert arriving live. Show history filters and Docker running locally.
Code walkthrough, 12 to 18 min. Show real code: project structure, containment logic, entry and exit detection, alert delivery, database schema, and how the frontend receives alerts.
Gaps and roadmap, 10 to 15 min. Be honest about what is missing and how long it would take. Explain how you would scale to thousands of vehicles and geofences. Reviewers score self-awareness here.
5. Questions asked after the assignment
These come from past MapUp interviews. Most of the later rounds are about defending your own submission, so prepare answers using your own project.
Area
Questions
Your assignment
Walk through it module by module. Why this folder structure? Trace a request from frontend to backend to database and back. Why this tech stack, and what alternatives did you reject? How did you handle validation and edge cases? What would you improve with more time? How did you use AI tools?
Geofencing
How do you define a geofence (polygon or radius) and store it? How do you detect entry and exit? How do you validate coordinates? How would you track vehicles in real time?
Go
What are goroutines and channels? What is a mutex and why is it needed? How do you prevent race conditions? How do you handle concurrency and errors in your API?
Database and performance
What is indexing and how does it speed up queries? How would you optimise API response time? How do caching and pagination help? How do you handle many concurrent requests?
Deployment
How did you containerise the backend? Where is it hosted and how do the frontend, backend and database connect? How do users get updated JavaScript files after a new deployment?
React and JavaScript
useMemo vs useCallback vs React.memo. How do you stop unnecessary re-renders and render large lists? var, let and const; == vs ===; closures; hoisting; promises and async/await.


6. Before you submit
Every endpoint returns the documented shape and time_ns.
An invalid geofence returns 400 with a clear message.
Both entry and exit trigger alerts, and two open browser tabs both receive them.
docker compose up works from a fresh clone.
Deployed frontend and backend talk to each other.
Collaborators added, video link in the README, form submitted.

All the best.
Scaler Placement Team
